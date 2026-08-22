package imports

import (
	"fmt"
	"io"
	"strings"
	"time"

	"finances/internal/txns"
	"github.com/extrame/xls"
)

// Column indices in the UOB card statement's transaction rows.
const (
	uobColTxDate   = 0
	uobColDesc     = 2
	uobColCurrency = 5
	uobColAmount   = 6
)

// ParseUOBCard parses a UOB credit-card statement (legacy BIFF8 .xls) into
// transactions. It needs an io.ReadSeeker because the xls reader seeks within
// the compound-document structure.
func ParseUOBCard(r io.ReadSeeker) ([]txns.Transaction, error) {
	wb, err := xls.OpenReader(r, "utf-8")
	if err != nil {
		return nil, err
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, fmt.Errorf("no sheet 0")
	}

	var rows []txns.Transaction
	account := "UOB"
	started := false
	for i := 0; i <= int(sheet.MaxRow); i++ {
		row := sheet.Row(i)
		if row == nil {
			continue
		}
		col := func(c int) string { return strings.TrimSpace(row.Col(c)) }

		// Everything before the "Transaction Date" header is statement metadata.
		if !started {
			if col(0) == "Account Type:" {
				account = col(1)
			}
			if col(0) == "Transaction Date" {
				started = true
			}
			continue
		}

		raw := col(uobColTxDate)
		if raw == "" { // "Previous Balance" pseudo-row and any trailing blanks
			continue
		}
		date, err := time.Parse("2 Jan 2006", raw)
		if err != nil {
			return nil, fmt.Errorf("row %d: bad date %q: %w", i, raw, err)
		}

		// UOB sign convention: positive = charge (spend), negative = payment.
		// transactions table uses the opposite: negative = spend, so flip.
		amt, err := parseAmt(col(uobColAmount))
		if err != nil {
			return nil, fmt.Errorf("row %d: bad amount %q: %w", i, col(uobColAmount), err)
		}

		ref := collapse(col(uobColDesc))

		currency := col(uobColCurrency) // Local Currency Type
		if currency == "" {
			currency = "SGD"
		}

		rows = append(rows, txns.Transaction{
			Date:      date.Format("2006-01-02"),
			Account:   txns.ShortName(account),
			Reference: &ref,
			Amount:    -amt,
			Currency:  currency,
		})
	}
	return rows, nil
}
