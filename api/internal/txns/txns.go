// Package txns owns the connection to the Supabase Postgres database and the
// shared helpers for reading and writing the transactions table. Both the API
// server and the statement-import commands use it.
package txns

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB is the shared database handle, set by Open.
var DB *sql.DB

// Transaction mirrors a row of the transactions table. Category, Reference,
// Owner and OwnerSource are pointers so they can be NULL.
type Transaction struct {
	Date      string  `json:"date"`
	Account   string  `json:"account"`
	Category  *string `json:"category"`
	Reference *string `json:"reference"`
	Amount    float64 `json:"amount"` // negative = spend, positive = credit
	Currency  string  `json:"currency"`
	Owner     *string `json:"owner"` // whose personal budget it falls in; nil = joint
	// OwnerSource records who decided Owner: "rule" if owner_rules did, "manual"
	// if a person did, nil if nobody has. It is derived by the server (see
	// ResolveOwnerSources) and ignored on input.
	OwnerSource *string `json:"ownerSource"`
}

// Open connects to the Postgres database named by DATABASE_URL, falling back to
// the value in a dotenv file beside the binary (.env, or api/.env when run from
// the repo root) so the import commands work without exporting anything. It
// pings the server, since a bad connection string is otherwise only discovered
// on the first request. Callers should defer Close.
func Open() error {
	url := DatabaseURL()
	if url == "" {
		return fmt.Errorf("DATABASE_URL is not set; copy api/.env.example to api/.env and fill in the password")
	}

	var err error
	DB, err = sql.Open("pgx", url)
	if err != nil {
		return err
	}

	// The Supabase pooler hands out a limited number of backends, and this is a
	// two-person app: a small pool that recycles is plenty, and keeps a stale
	// connection from outliving a pooler restart.
	DB.SetMaxOpenConns(10)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxIdleTime(5 * time.Minute)
	DB.SetConnMaxLifetime(30 * time.Minute)

	if err := DB.Ping(); err != nil {
		DB.Close()
		DB = nil
		return fmt.Errorf("connecting to Postgres: %w", err)
	}
	return nil
}

// DatabaseURL returns the connection string from the environment, else from a
// dotenv file, else "".
func DatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	for _, path := range []string{".env", "api/.env"} {
		if url := fromEnvFile(path, "DATABASE_URL"); url != "" {
			return url
		}
	}
	return ""
}

// fromEnvFile returns the value of key in a dotenv-style file, or "" if the
// file or the key is missing. It handles KEY=value lines with optional quotes;
// anything fancier belongs in the real environment.
func fromEnvFile(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}

// Close closes the shared handle if it is open.
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// rebind rewrites the ? placeholders this package's queries are written with
// into the $1, $2 … form Postgres wants. Queries are assembled from fragments —
// a date filter here, an owner filter there, a LIMIT on the end — and numbering
// those by hand as they are concatenated is how off-by-one bugs get in.
// Anything inside a quoted string or identifier is left alone.
func rebind(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 8)

	n := 0
	for i := 0; i < len(query); i++ {
		switch c := query[i]; c {
		case '\'', '"':
			// Copy the literal whole, including a doubled quote that escapes
			// itself ('' inside '…'), so a ? within it is not a placeholder.
			b.WriteByte(c)
			for i++; i < len(query); i++ {
				b.WriteByte(query[i])
				if query[i] == c {
					if i+1 < len(query) && query[i+1] == c {
						i++
						b.WriteByte(query[i])
						continue
					}
					break
				}
			}
		case '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Exec runs a statement written with ? placeholders.
func Exec(q string, args ...any) (sql.Result, error) {
	return DB.Exec(rebind(q), args...)
}

// normalize converts a Postgres value into the plain Go type the rest of the
// app — and the JSON it hands the UI — expects. Two column types need it: DATE
// arrives as a time.Time and would be marshalled as a full RFC 3339 timestamp
// rather than the YYYY-MM-DD the API has always returned, and NUMERIC arrives
// as its decimal string, which would turn every amount in the JSON into a
// string and every SUM into 0 by the time num got hold of it.
func normalize(dbType string, v any) any {
	if v == nil {
		return nil
	}
	switch dbType {
	case "DATE":
		if t, ok := v.(time.Time); ok {
			return t.Format("2006-01-02")
		}
	case "NUMERIC":
		switch n := v.(type) {
		case string:
			f, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return n
			}
			return f
		case []byte:
			f, err := strconv.ParseFloat(string(n), 64)
			if err != nil {
				return string(n)
			}
			return f
		}
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// QueryRows runs a query written with ? placeholders and returns each row as a
// column->value map.
func QueryRows(q string, args ...any) ([]map[string]any, error) {
	rows, err := DB.Query(rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(types))
		ptrs := make([]any, len(types))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(types))
		for i, t := range types {
			row[t.Name()] = normalize(t.DatabaseTypeName(), vals[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// QueryOne runs a query and returns the first row, or nil if there are none.
func QueryOne(q string, args ...any) (map[string]any, error) {
	rows, err := QueryRows(q, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// insertColumns is the column list Insert writes, and insertChunk how many rows
// it sends per statement. The database is now across a network, so rows go up
// in batches rather than one round trip each; Postgres caps a statement at
// 65535 parameters, which this stays well inside.
var insertColumns = []string{"date", "account", "category", "reference", "amount", "currency", "owner", "owner_source"}

const insertChunk = 500

// Insert writes all transactions in a single database transaction. Empty
// currency defaults to SGD. Returns the number of rows inserted.
func Insert(rows []Transaction) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	for start := 0; start < len(rows); start += insertChunk {
		end := min(start+insertChunk, len(rows))
		batch := rows[start:end]

		args := make([]any, 0, len(batch)*len(insertColumns))
		values := make([]string, 0, len(batch))
		n := 0
		for _, t := range batch {
			currency := t.Currency
			if currency == "" {
				currency = "SGD"
			}
			placeholders := make([]string, len(insertColumns))
			for i := range placeholders {
				n++
				placeholders[i] = "$" + strconv.Itoa(n)
			}
			values = append(values, "("+strings.Join(placeholders, ", ")+")")
			args = append(args, t.Date, t.Account, t.Category, t.Reference, t.Amount, currency, t.Owner, t.OwnerSource)
		}

		q := "INSERT INTO transactions (" + strings.Join(insertColumns, ", ") + ") VALUES " + strings.Join(values, ", ")
		if _, err := tx.Exec(q, args...); err != nil {
			return 0, ownerFKError(err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// CategorizeRows assigns a suggested category to each uncategorized transaction
// whose reference contains one of the category_keywords, mutating rows in place.
// When several keywords match the same reference, the longest (most specific)
// one wins. It mirrors Categorize but works on in-memory rows that have not yet
// been written to the database. Returns the number of rows categorized.
func CategorizeRows(rows []Transaction) (int, error) {
	keywords, err := QueryRows(
		"SELECT keyword, category FROM category_keywords ORDER BY LENGTH(keyword) DESC",
	)
	if err != nil {
		return 0, err
	}

	n := 0
	for i := range rows {
		if rows[i].Category != nil || rows[i].Reference == nil {
			continue
		}
		ref := strings.ToUpper(*rows[i].Reference)
		for _, kw := range keywords {
			keyword, _ := kw["keyword"].(string)
			category, _ := kw["category"].(string)
			if keyword == "" || category == "" {
				continue
			}
			if strings.Contains(ref, strings.ToUpper(keyword)) {
				cat := category
				rows[i].Category = &cat
				n++
				break
			}
		}
	}
	return n, nil
}

// shortNames maps the account label as it appears in a statement to the
// canonical short name already used in the database. A statement writes the
// account number or card PAN after the product name, so the label is matched by
// its leading product name rather than in full — the identifier varies per
// account and does not belong in the source tree. Longest prefix first, so a
// more specific product wins over one whose name is a prefix of it.
var shortNames = []struct{ prefix, short string }{
	{"UOB PRVI MILES VISA CARD", "UOB PRVI MILES"},
	{"DBS MasterCard World", "DBS MCW"},
	{"Joint Budgeting", "Joint Budgeting"},
	{"Personal Main", "Personal Main"},
}

// ShortName returns the canonical short account name for a statement's account
// label, or the label unchanged if there's no known mapping.
func ShortName(account string) string {
	for _, n := range shortNames {
		if strings.HasPrefix(account, n.prefix) {
			return n.short
		}
	}
	return account
}
