// Command schemacheck compares the database's actual shape against what this
// codebase assumes about it, and prints the differences. It is a read-only
// counterpart to pgcheck: where that one proves a connection string reaches a
// server, this one proves the server still holds the tables and columns the
// queries are written against. If DATABASE_URL is unset the value is read from
// api/.env.
//
//	go run ./cmd/schemacheck
//
// The expectations below are not a migration or a source of truth for the
// schema — the schema is managed in Supabase. They are a transcription of what
// the Go code would break on, so that editing a table there and forgetting to
// edit a query here shows up as a line of output rather than as a 500 at
// runtime. A column the code has never heard of is reported too, since that is
// usually the half of a schema change still waiting to be wired up.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"finances/internal/txns"
)

// column is one column the code depends on. dataType is the information_schema
// name, and matters beyond documentation for two of them: txns.normalize keys
// off DATE and NUMERIC to turn a date into the YYYY-MM-DD string and an amount
// into a float on the way out, so a column that quietly becomes timestamptz or
// float8 keeps answering queries while changing the JSON the UI is handed.
type column struct {
	name     string
	dataType string
	// nullable records whether the code ever puts a NULL here. A NOT NULL added
	// to one of these in Supabase breaks the next insert rather than the next
	// query, which is worth hearing about before an import run rather than
	// during one.
	nullable bool
}

type table struct {
	name    string
	columns []column
}

// expected is every table and column the queries in this repo name. Sources:
// txns.insertColumns and the Transaction struct (transactions), the dashboard
// and category handlers in main.go (categories), CategorizeRows
// (category_keywords), loadOwnerRules (owner_rules) and loadBudgetContributors
// (budget_contributors).
var expected = []table{
	{"transactions", []column{
		{"id", "integer", false},
		{"date", "date", false},
		{"account", "text", false},
		{"category", "text", true},
		{"reference", "text", true},
		{"amount", "numeric", false},
		{"currency", "text", false},
		{"owner", "text", true},
		{"owner_source", "text", true},
	}},
	{"categories", []column{
		{"name", "text", false},
		{"cadence", "text", false},
	}},
	{"category_keywords", []column{
		{"keyword", "text", false},
		{"category", "text", false},
	}},
	{"owner_rules", []column{
		{"keyword", "text", false},
		{"category", "text", true},
		{"owner", "text", false},
	}},
	{"budget_contributors", []column{
		{"name", "text", false},
		{"reference_match", "text", false},
		{"expected", "numeric", false},
	}},
}

// actualColumn is what information_schema.columns says about one column.
type actualColumn struct {
	dataType string
	nullable bool
}

// interchangeable groups the information_schema type names the code treats
// alike, so a bigint id or a varchar reference is not reported as drift. The
// integer family is one group because every width reaches the handlers through
// QueryRows as an int64; the string family is one because text and varchar both
// scan into a string. date and numeric are deliberately absent — those are the
// two normalize keys off, so nothing substitutes for them.
var interchangeable = map[string]string{
	"smallint": "integer", "integer": "integer", "bigint": "integer",
	"text": "text", "character varying": "text", "character": "text",
}

func family(dataType string) string {
	if f, ok := interchangeable[dataType]; ok {
		return f
	}
	return dataType
}

func main() {
	if err := txns.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer txns.Close()

	// Every column of every relation in the public schema, rather than only the
	// tables named above: a schema change often arrives as a whole new table,
	// and one this command had no expectation for would otherwise read as no
	// change at all. table_type comes along so a new relation can be reported
	// as the table or view it actually is.
	rows, err := txns.QueryRows(
		`SELECT c.table_name, c.column_name, c.data_type, c.is_nullable, t.table_type
		   FROM information_schema.columns c
		   JOIN information_schema.tables t
		     ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		  WHERE c.table_schema = 'public'`,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading information_schema: %v\n", err)
		os.Exit(1)
	}

	actual := make(map[string]map[string]actualColumn, len(expected))
	relKind := map[string]string{}
	for _, row := range rows {
		t, _ := row["table_name"].(string)
		c, _ := row["column_name"].(string)
		d, _ := row["data_type"].(string)
		n, _ := row["is_nullable"].(string)
		k, _ := row["table_type"].(string)
		if actual[t] == nil {
			actual[t] = map[string]actualColumn{}
		}
		actual[t][c] = actualColumn{dataType: d, nullable: n == "YES"}
		relKind[t] = strings.ToLower(k)
	}

	var breaking, notes []string
	for _, t := range expected {
		cols, ok := actual[t.name]
		if !ok {
			breaking = append(breaking, fmt.Sprintf("table %s is missing", t.name))
			continue
		}
		known := make(map[string]bool, len(t.columns))
		for _, want := range t.columns {
			known[want.name] = true
			got, ok := cols[want.name]
			if !ok {
				breaking = append(breaking, fmt.Sprintf("%s.%s is missing, and the code reads or writes it", t.name, want.name))
				continue
			}
			if family(got.dataType) != family(want.dataType) {
				breaking = append(breaking, fmt.Sprintf("%s.%s is %s, the code expects %s", t.name, want.name, got.dataType, want.dataType))
			}
			if want.nullable && !got.nullable {
				breaking = append(breaking, fmt.Sprintf("%s.%s is NOT NULL, but the code writes NULL to it", t.name, want.name))
			}
		}
		added := make([]string, 0, len(cols))
		for name, got := range cols {
			if !known[name] {
				added = append(added, fmt.Sprintf("%s.%s (%s) exists, but no query names it", t.name, name, got.dataType))
			}
		}
		sort.Strings(added)
		notes = append(notes, added...)
	}

	// A relation the expectations above say nothing about. Not a failure — the
	// database is allowed to hold things this app does not read — but it is the
	// shape a new feature's table arrives in, so it is worth naming.
	expectedTable := make(map[string]bool, len(expected))
	for _, t := range expected {
		expectedTable[t.name] = true
	}
	unknown := make([]string, 0, len(actual))
	for name, cols := range actual {
		if expectedTable[name] {
			continue
		}
		kind := relKind[name]
		if kind == "base table" {
			kind = "table"
		}
		unknown = append(unknown, fmt.Sprintf("%s %s (%d columns) exists, but no query names it", kind, name, len(cols)))
	}
	sort.Strings(unknown)
	notes = append(notes, unknown...)

	for _, line := range breaking {
		fmt.Printf("BREAK  %s\n", line)
	}
	for _, line := range notes {
		fmt.Printf("NEW    %s\n", line)
	}
	switch {
	case len(breaking) > 0:
		fmt.Printf("\n%d breaking difference(s), %d column(s) the code does not use.\n", len(breaking), len(notes))
		os.Exit(1)
	case len(notes) > 0:
		fmt.Printf("\nEvery query is satisfied; %d column(s) the code does not use yet.\n", len(notes))
	default:
		fmt.Println("Schema matches what the code expects.")
	}
}
