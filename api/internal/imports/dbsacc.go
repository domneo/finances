package imports

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"finances/internal/txns"
)

// Column indices in the DBS account statement's transaction rows.
const (
	accColTxDate   = 0
	accColDesc     = 3
	accColSuppDesc = 5
	accColCurrency = 9
	accColDebit    = 10
	accColCredit   = 11
)

// ParseDBSAccount parses a DBS account statement CSV into transactions.
func ParseDBSAccount(r io.Reader) ([]txns.Transaction, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // metadata rows vary in width

	var rows []txns.Transaction
	account := "DBS"
	started := false
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		field := func(i int) string { return cell(rec, i) }

		// Everything before the "Transaction Date" header is statement metadata.
		if !started {
			if field(0) == "Account Details For:" {
				account = field(1)
			}
			if field(0) == "Transaction Date" {
				started = true
			}
			continue
		}

		raw := field(accColTxDate)
		if raw == "" {
			continue
		}
		date, err := time.Parse("2 Jan 2006", raw)
		if err != nil {
			return nil, fmt.Errorf("bad date %q: %w", raw, err)
		}

		// Debit = money out (spend), Credit = money in. The transactions table
		// uses negative = spend, positive = credit, so amount = credit - debit.
		debit, err := parseAmt(field(accColDebit))
		if err != nil {
			return nil, fmt.Errorf("bad debit amount: %w", err)
		}
		credit, err := parseAmt(field(accColCredit))
		if err != nil {
			return nil, fmt.Errorf("bad credit amount: %w", err)
		}
		amount := credit - debit

		// Description is the reference; fall back to the supplementary
		// description (e.g. interest rows have a blank Description).
		ref := collapse(field(accColDesc))
		if ref == "" {
			ref = collapse(field(accColSuppDesc))
		}

		currency := field(accColCurrency)
		if currency == "" {
			currency = "SGD"
		}

		rows = append(rows, txns.Transaction{
			Date:      date.Format("2006-01-02"),
			Account:   txns.ShortName(account),
			Reference: &ref,
			Amount:    amount,
			Currency:  currency,
		})
	}
	return rows, nil
}
