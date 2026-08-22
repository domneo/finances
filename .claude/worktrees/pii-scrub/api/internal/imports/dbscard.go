package imports

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"finances/internal/txns"
)

// Column indices in the DBS card statement's transaction rows.
const (
	cardColTxDate = 0
	cardColDesc   = 2
	cardColDebit  = 6
	cardColCredit = 7
)

// ParseDBSCard parses a DBS credit-card statement CSV into transactions.
func ParseDBSCard(r io.Reader) ([]txns.Transaction, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // metadata rows vary in width

	var rows []txns.Transaction
	account := "DBS Card"
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
			if field(0) == "Card Transaction Details For:" {
				account = field(1)
			}
			if field(0) == "Transaction Date" {
				started = true
			}
			continue
		}

		raw := field(cardColTxDate)
		if raw == "" {
			continue
		}
		date, err := time.Parse("2 Jan 2006", raw)
		if err != nil {
			return nil, fmt.Errorf("bad date %q: %w", raw, err)
		}

		// Debit = charge (spend), Credit = payment. The transactions table uses
		// negative = spend, positive = credit, so amount = credit - debit.
		debit, err := parseAmt(field(cardColDebit))
		if err != nil {
			return nil, fmt.Errorf("bad debit amount: %w", err)
		}
		credit, err := parseAmt(field(cardColCredit))
		if err != nil {
			return nil, fmt.Errorf("bad credit amount: %w", err)
		}
		amount := credit - debit

		ref := collapse(field(cardColDesc))

		rows = append(rows, txns.Transaction{
			Date:      date.Format("2006-01-02"),
			Account:   txns.ShortName(account),
			Reference: &ref,
			Amount:    amount,
			Currency:  "SGD", // no currency column in the card export
		})
	}
	return rows, nil
}
