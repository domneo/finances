// Command dbsaccconv imports a DBS account statement CSV directly into the
// transactions table in the Supabase Postgres database named by DATABASE_URL.
//
//	go run ./cmd/dbsaccconv transactions/dbs/dbs-personal-202605.csv
package main

import (
	"fmt"
	"os"

	"finances/internal/imports"
	"finances/internal/txns"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dbsaccconv <file.csv>")
		os.Exit(2)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	rows, err := imports.ParseDBSAccount(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	if err := txns.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer txns.Close()

	n, err := txns.Insert(rows)
	if err != nil {
		fmt.Fprintf(os.Stderr, "insert: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("inserted %d transactions\n", n)
}
