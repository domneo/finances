package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"finances/internal/imports"
	"finances/internal/txns"
)

func main() {
	if err := txns.Open(); err != nil {
		log.Fatal(err)
	}
	defer txns.Close()

	// More specific paths must be registered first.
	http.HandleFunc("/api/dashboard", dashboardHandler)
	http.HandleFunc("/api/transactions/summary", summaryHandler)
	http.HandleFunc("/api/transactions/monthly", monthlyHandler)
	http.HandleFunc("/api/transactions/import", importTransactionsHandler)
	http.HandleFunc("/api/transactions/", transactionByIDHandler)
	http.HandleFunc("/api/transactions", transactionsHandler)
	http.HandleFunc("/api/categories", categoriesHandler)
	http.HandleFunc("/api/accounts", accountsHandler)
	http.HandleFunc("/api/owners", ownersHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5001"
	}
	log.Printf("listening on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	// Naming an owner who is not a budget contributor is the client's mistake,
	// not the server's: the owner columns are foreign keys to
	// budget_contributors, so the database is the thing that catches it, and
	// without this the caller would get a 500 quoting a constraint name.
	if errors.Is(err, txns.ErrUnknownOwner) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func dateFilters(r *http.Request) (clauses []string, args []any) {
	if v := r.URL.Query().Get("from"); v != "" {
		clauses = append(clauses, "date >= ?")
		args = append(args, v)
	}
	if v := r.URL.Query().Get("to"); v != "" {
		clauses = append(clauses, "date <= ?")
		args = append(args, v)
	}
	return
}

// ownerFilter narrows a query to one partner's personal budget. The reserved
// value "joint" selects everything that belongs to neither — the shared
// household spending — which is otherwise unreachable, since it is stored as NULL.
func ownerFilter(r *http.Request, clauses []string, args []any) ([]string, []any) {
	v := r.URL.Query().Get("owner")
	switch v {
	case "":
	case "joint":
		clauses = append(clauses, "owner IS NULL")
	default:
		clauses = append(clauses, "owner = ?")
		args = append(args, v)
	}
	return clauses, args
}

func whereClause(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(clauses, " AND ")
}

func transactionByIDHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/transactions/")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "invalid transaction id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		patchTransactionHandler(w, r, id)
	case http.MethodDelete:
		res, err := txns.Exec("DELETE FROM transactions WHERE id = ?", id)
		if err != nil {
			writeError(w, err)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			http.Error(w, "transaction not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"deleted": id})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// patchTransactionHandler edits one transaction. Only "owner" is editable:
// send the partner's name to move the transaction into their personal budget,
// or null to return it to the joint one. Sending no owner field at all is a
// no-op rather than an accidental reset.
func patchTransactionHandler(w http.ResponseWriter, r *http.Request, id int) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	raw, ok := body["owner"]
	if !ok {
		http.Error(w, "nothing to update", http.StatusBadRequest)
		return
	}
	var owner *string
	if err := json.Unmarshal(raw, &owner); err != nil {
		http.Error(w, "owner must be a string or null", http.StatusBadRequest)
		return
	}

	found, err := txns.SetOwner(id, owner)
	if err != nil {
		writeError(w, err)
		return
	}
	if !found {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}

	row, err := txns.QueryOne("SELECT * FROM transactions WHERE id = ?", id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, row)
}

func transactionsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getTransactionsHandler(w, r)
	case http.MethodPost:
		addTransactionsHandler(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query()
	clauses, args := dateFilters(r)

	if v := p.Get("account"); v != "" {
		clauses = append(clauses, "account = ?")
		args = append(args, v)
	}
	if v := p.Get("category"); v != "" {
		clauses = append(clauses, "category = ?")
		args = append(args, v)
	}
	clauses, args = ownerFilter(r, clauses, args)

	where := whereClause(clauses)

	limit := 100
	if v := p.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	if v := p.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// id breaks the tie between transactions on the same date. Without it the
	// order within a date is whatever Postgres finds convenient, which is not
	// stable between queries — so a row could be shown on two consecutive pages
	// while another was skipped entirely.
	rows, err := txns.QueryRows(
		"SELECT * FROM transactions "+where+" ORDER BY date DESC, id DESC LIMIT ? OFFSET ?",
		append(args, limit, offset)...,
	)
	if err != nil {
		writeError(w, err)
		return
	}

	total, err := txns.QueryOne("SELECT COUNT(*) AS n FROM transactions "+where, args...)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, map[string]any{
		"total":  total["n"],
		"limit":  limit,
		"offset": offset,
		"data":   rows,
	})
}

func addTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	var items []txns.Transaction
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(items) == 0 {
		writeJSON(w, map[string]any{"inserted": 0})
		return
	}

	// The rows have been through a person's hands, so record whether each
	// owner is still the rules' choice or that person's own.
	if err := txns.ResolveOwnerSources(items); err != nil {
		writeError(w, err)
		return
	}

	n, err := txns.Insert(items)
	if err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"inserted": n})
}

// importTransactionsHandler accepts a multipart upload of a raw bank statement
// (field "file"), auto-detects its format from the contents, parses it, and
// returns the resulting transactions with suggested categories for the user to
// review and edit. It does NOT write to the database — the reviewed rows are
// committed separately via POST /api/transactions.
func importTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	rows, err := imports.Parse(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rows == nil {
		rows = []txns.Transaction{}
	}

	categorized, err := txns.CategorizeRows(rows)
	if err != nil {
		writeError(w, err)
		return
	}

	// Owner rules can be pinned to a category, so this has to follow the
	// categorization above.
	owned, err := txns.AssignOwners(rows)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, map[string]any{
		"transactions": rows,
		"count":        len(rows),
		"categorized":  categorized,
		"owned":        owned,
	})
}

func summaryHandler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query()
	clauses, args := dateFilters(r)
	if v := p.Get("account"); v != "" {
		clauses = append(clauses, "account = ?")
		args = append(args, v)
	}
	clauses, args = ownerFilter(r, clauses, args)
	where := whereClause(clauses)

	rows, err := txns.QueryRows(
		`SELECT category, SUM(amount) AS total, COUNT(*) AS count
		 FROM transactions `+where+`
		 GROUP BY category
		 ORDER BY total ASC`,
		args...,
	)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, rows)
}

func monthlyHandler(w http.ResponseWriter, r *http.Request) {
	clauses, args := dateFilters(r)
	clauses, args = ownerFilter(r, clauses, args)
	where := whereClause(clauses)

	rows, err := txns.QueryRows(
		`SELECT to_char(date, 'YYYY-MM') AS month,
		        SUM(CASE WHEN amount < 0 THEN amount ELSE 0 END) AS spend,
		        SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END) AS income,
		        SUM(amount) AS net
		 FROM transactions `+where+`
		 GROUP BY month
		 ORDER BY month ASC`,
		args...,
	)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, rows)
}

func categoriesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := txns.QueryRows("SELECT name, cadence FROM categories ORDER BY name")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, rows)
}

// budgetAccount holds the pooled monthly spending budget: both partners pay
// their fixed contribution into it by standing instruction.
const budgetAccount = "Joint Budgeting"

// budgetContributor is one fixed monthly contribution that makes up the budget.
// A contribution is recognised as a credit into budgetAccount whose reference
// contains ReferenceMatch (the payer's name, as the bank writes it), and is
// reported under the shorter Name.
type budgetContributor struct {
	Name           string
	ReferenceMatch string
	Expected       float64
}

// loadBudgetContributors reads budget_contributors, the way owner rules and
// category keywords are read. A bank names a payer in full, and a legal name
// and a salary are personal data, so the contributors live in the database
// beside the transactions they match rather than in the source tree.
func loadBudgetContributors() ([]budgetContributor, error) {
	rows, err := txns.QueryRows(
		"SELECT name, reference_match, expected FROM budget_contributors ORDER BY name",
	)
	if err != nil {
		return nil, err
	}

	contributors := make([]budgetContributor, 0, len(rows))
	for _, row := range rows {
		name, _ := row["name"].(string)
		match, _ := row["reference_match"].(string)
		if name == "" || match == "" {
			continue
		}
		contributors = append(contributors, budgetContributor{
			Name:           name,
			ReferenceMatch: match,
			Expected:       num(row["expected"]),
		})
	}
	return contributors, nil
}

// recurringWindow is how many months of history the typical amount of a
// recurring expense is averaged over. Twelve covers exactly one yearly bill.
const recurringWindow = 12

type contribution struct {
	Name     string  `json:"name"`
	Expected float64 `json:"expected"`
	Received float64 `json:"received"`
	Count    int64   `json:"count"`
	Date     string  `json:"date"` // "" when nothing landed this month
}

type recurring struct {
	Category string  `json:"category"`
	Cadence  string  `json:"cadence"`
	Monthly  float64 `json:"monthly"` // monthly-equivalent flow, negative = spend
	Actual   float64 `json:"actual"`  // net movement in the selected month
	Count    int64   `json:"count"`   // transactions in the selected month
	LastPaid string  `json:"lastPaid"`
}

type categoryTotal struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
	Count    int64   `json:"count"`
}

// dashboardHandler assembles the landing page: the pooled budget and who has
// paid into it, the recurring expenses with their monthly-equivalent cost and
// whether each has been charged, and the variable spend per category — all for
// a single month (?month=YYYY-MM, defaulting to the latest month with data).
//
// This is the joint budget, so every query below is restricted to owner IS
// NULL. A transaction claimed by a partner comes out of their own budget, and
// counting it here would overstate what the household spends — and, for a
// recurring category, drag its monthly-equivalent along with it.
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	if month == "" {
		row, err := txns.QueryOne("SELECT to_char(MAX(date), 'YYYY-MM') AS month FROM transactions")
		if err != nil {
			writeError(w, err)
			return
		}
		month = str(row["month"])
	}
	if month == "" { // no transactions at all
		writeJSON(w, map[string]any{
			"month":     "",
			"budget":    []contribution{},
			"recurring": []recurring{},
			"variable":  []categoryTotal{},
		})
		return
	}
	start, err := time.Parse("2006-01", month)
	if err != nil {
		http.Error(w, "invalid month, want YYYY-MM", http.StatusBadRequest)
		return
	}
	monthEnd := start.AddDate(0, 1, -1).Format("2006-01-02")
	windowStart := start.AddDate(0, -(recurringWindow - 1), 0).Format("2006-01")

	budget, err := budgetContributions(month)
	if err != nil {
		writeError(w, err)
		return
	}
	bills, err := recurringExpenses(month, windowStart, monthEnd)
	if err != nil {
		writeError(w, err)
		return
	}
	variable, err := variableExpenses(month)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, map[string]any{
		"month":     month,
		"budget":    budget,
		"recurring": bills,
		"variable":  variable,
	})
}

// budgetContributions reports, per contributor, what landed in the budget
// account this month against what was expected.
func budgetContributions(month string) ([]contribution, error) {
	contributors, err := loadBudgetContributors()
	if err != nil {
		return nil, err
	}

	out := make([]contribution, 0, len(contributors))
	for _, c := range contributors {
		row, err := txns.QueryOne(
			`SELECT COALESCE(SUM(amount), 0) AS received,
			        COUNT(*) AS count,
			        MIN(date) AS date
			 FROM transactions
			 WHERE to_char(date, 'YYYY-MM') = ?
			   AND account = ?
			   AND owner IS NULL
			   AND amount > 0
			   AND UPPER(reference) LIKE ?`,
			month, budgetAccount, "%"+strings.ToUpper(c.ReferenceMatch)+"%",
		)
		if err != nil {
			return nil, err
		}
		out = append(out, contribution{
			Name:     c.Name,
			Expected: c.Expected,
			Received: num(row["received"]),
			Count:    int64(num(row["count"])),
			Date:     str(row["date"]),
		})
	}
	return out, nil
}

// recurringExpenses lists every monthly and yearly category with its
// monthly-equivalent flow. Monthly ones are averaged over the months they were
// actually active in the trailing window; yearly ones are the window total
// spread over 12. Categories with no history at all are still listed, at zero.
//
// Nothing declares which way a category runs, so the typical amount is built
// from the flow it actually moves in — charges for a bill, receipts for an
// income stream. The opposite flow (a refund on a bill, a clawback on income)
// is a one-off, and letting it into the average would misstate what the
// category normally does. This month's actual is the net figure, so a refund
// does reduce what was spent.
func recurringExpenses(month, windowStart, monthEnd string) ([]recurring, error) {
	cats, err := txns.QueryRows(
		`SELECT name, cadence FROM categories
		 WHERE cadence IN ('monthly', 'yearly')
		 ORDER BY cadence, name`,
	)
	if err != nil {
		return nil, err
	}

	// One row per category per month of the window, split by flow so a bill can
	// be averaged over the months it was actually charged in — and an income
	// stream over the months it actually paid. `total` keeps both, for the actual.
	window, err := txns.QueryRows(
		`SELECT t.category AS category,
		        to_char(t.date, 'YYYY-MM') AS month,
		        SUM(t.amount) AS total,
		        SUM(CASE WHEN t.amount < 0 THEN t.amount ELSE 0 END) AS outflow,
		        SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END) AS inflow,
		        COUNT(*) AS count
		 FROM transactions t JOIN categories c ON c.name = t.category
		 WHERE c.cadence IN ('monthly', 'yearly')
		   AND t.owner IS NULL
		   AND to_char(t.date, 'YYYY-MM') BETWEEN ? AND ?
		 GROUP BY t.category, month`,
		windowStart, month,
	)
	if err != nil {
		return nil, err
	}

	// Last payment date can predate the window (a yearly bill missed this year).
	lastRows, err := txns.QueryRows(
		`SELECT category, MAX(date) AS last FROM transactions
		 WHERE category IS NOT NULL AND owner IS NULL AND date <= ?
		 GROUP BY category`,
		monthEnd,
	)
	if err != nil {
		return nil, err
	}
	lastPaid := make(map[string]string, len(lastRows))
	for _, row := range lastRows {
		lastPaid[str(row["category"])] = str(row["last"])
	}

	type monthFlow struct{ inflow, outflow float64 }
	type agg struct {
		months []monthFlow
		net    float64 // window net; its sign is the way the category runs
		actual float64
		count  int64
	}
	totals := make(map[string]*agg, len(cats))
	for _, row := range window {
		cat := str(row["category"])
		a := totals[cat]
		if a == nil {
			a = &agg{}
			totals[cat] = a
		}
		f := monthFlow{inflow: num(row["inflow"]), outflow: num(row["outflow"])}
		a.months = append(a.months, f)
		a.net += f.inflow + f.outflow
		if str(row["month"]) == month {
			a.actual = num(row["total"])
			a.count = int64(num(row["count"]))
		}
	}

	out := make([]recurring, 0, len(cats))
	for _, c := range cats {
		name, cadence := str(c["name"]), str(c["cadence"])
		a := totals[name]
		if a == nil {
			a = &agg{}
		}
		// A month that only saw the opposite flow — a refund on a bill — was never
		// charged, so it neither adds to the window nor counts against the average.
		windowTotal, activeMonths := 0.0, 0
		for _, f := range a.months {
			flow := f.outflow
			if a.net > 0 {
				flow = f.inflow
			}
			if flow != 0 {
				windowTotal += flow
				activeMonths++
			}
		}
		monthly := 0.0
		switch {
		case cadence == "yearly":
			monthly = windowTotal / recurringWindow
		case activeMonths > 0:
			monthly = windowTotal / float64(activeMonths)
		}
		out = append(out, recurring{
			Category: name,
			Cadence:  cadence,
			Monthly:  monthly,
			Actual:   a.actual,
			Count:    a.count,
			LastPaid: lastPaid[name],
		})
	}
	return out, nil
}

// variableExpenses nets each variable category for the month — spend comes back
// negative, an inflow positive, whichever way the month's transactions ran.
// Uncategorized transactions are reported alongside them so the gap is visible.
func variableExpenses(month string) ([]categoryTotal, error) {
	rows, err := txns.QueryRows(
		`SELECT c.name AS category,
		        COALESCE(SUM(t.amount), 0) AS total,
		        COUNT(t.id) AS count
		 FROM categories c
		 LEFT JOIN transactions t
		        ON t.category = c.name AND t.owner IS NULL
		       AND to_char(t.date, 'YYYY-MM') = ?
		 WHERE c.cadence = 'variable'
		 GROUP BY c.name`,
		month,
	)
	if err != nil {
		return nil, err
	}

	out := make([]categoryTotal, 0, len(rows)+1)
	for _, row := range rows {
		out = append(out, categoryTotal{
			Category: str(row["category"]),
			Total:    num(row["total"]),
			Count:    int64(num(row["count"])),
		})
	}

	un, err := txns.QueryOne(
		`SELECT COALESCE(SUM(amount), 0) AS total, COUNT(*) AS count
		 FROM transactions
		 WHERE category IS NULL AND owner IS NULL AND to_char(date, 'YYYY-MM') = ?`,
		month,
	)
	if err != nil {
		return nil, err
	}
	if n := int64(num(un["count"])); n > 0 {
		out = append(out, categoryTotal{Total: num(un["total"]), Count: n})
	}

	// Biggest spend first; negative amounts are spend, so ascending — which also
	// lands the categories that took money in at the bottom.
	sort.Slice(out, func(i, j int) bool { return out[i].Total < out[j].Total })
	return out, nil
}

// num coerces a scalar to a float. SUM returns NUMERIC and COUNT returns
// INT8; QueryRows has already turned the former into a float64 and the latter
// arrives as an int64, and both reach here as any.
func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	}
	return 0
}

// str coerces a scalar to a string, yielding "" for NULL — including the
// NULL an aggregate over no rows returns.
func str(v any) string {
	s, _ := v.(string)
	return s
}

// ownersHandler lists the partners who have a personal budget.
func ownersHandler(w http.ResponseWriter, r *http.Request) {
	owners, err := txns.Owners()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, owners)
}

func accountsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := txns.QueryRows("SELECT DISTINCT account FROM transactions ORDER BY account")
	if err != nil {
		writeError(w, err)
		return
	}
	accounts := make([]string, len(rows))
	for i, row := range rows {
		if v, ok := row["account"].(string); ok {
			accounts[i] = v
		}
	}
	writeJSON(w, accounts)
}
