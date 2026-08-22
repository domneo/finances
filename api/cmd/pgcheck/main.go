// Command pgcheck opens the shared connection to the Supabase Postgres
// database and prints the server version and what the app's tables hold, so a
// connection string can be verified before anything depends on it. It writes
// nothing. If DATABASE_URL is unset the value is read from api/.env.
//
//	go run ./cmd/pgcheck
package main

import (
	"fmt"
	"os"

	"finances/internal/txns"
)

func main() {
	if err := txns.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer txns.Close()

	row, err := txns.QueryOne("SELECT version() AS version, current_database() AS db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "query: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("connected to %v\n%v\n", row["db"], row["version"])

	counts, err := txns.QueryOne(
		`SELECT (SELECT COUNT(*) FROM transactions)      AS transactions,
		        (SELECT COUNT(*) FROM categories)        AS categories,
		        (SELECT COUNT(*) FROM category_keywords) AS category_keywords,
		        (SELECT COUNT(*) FROM owner_rules)       AS owner_rules`,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "counts: %v\n", err)
		os.Exit(1)
	}
	for _, name := range []string{"transactions", "categories", "category_keywords", "owner_rules"} {
		fmt.Printf("%-18s %v\n", name, counts[name])
	}
}
