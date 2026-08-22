// Package imports parses bank statement files (DBS account/card CSVs, UOB card
// xls) into transactions. Each parser is pure: it reads a stream and returns
// []txns.Transaction without touching the filesystem or database. Both the
// statement-import commands and the API upload endpoint use it.
package imports

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"finances/internal/txns"
)

// Format identifies a recognised statement layout.
type Format string

const (
	FormatDBSAccount Format = "dbs-account"
	FormatDBSCard    Format = "dbs-card"
	FormatUOBCard    Format = "uob-card"
	FormatUnknown    Format = ""
)

// ole2Magic is the compound-document (BIFF8 .xls) file signature.
var ole2Magic = []byte{0xD0, 0xCF, 0x11, 0xE0}

// Detect identifies the statement format from the file's contents, then seeks
// the reader back to the start so a parser can read it from the beginning.
func Detect(r io.ReadSeeker) (Format, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return FormatUnknown, err
	}
	head = head[:n]
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return FormatUnknown, err
	}

	switch {
	case bytes.HasPrefix(head, ole2Magic):
		// Only the UOB card statement is a binary .xls.
		return FormatUOBCard, nil
	case bytes.Contains(head, []byte("Account Details For:")):
		return FormatDBSAccount, nil
	case bytes.Contains(head, []byte("Card Transaction Details For:")):
		return FormatDBSCard, nil
	default:
		return FormatUnknown, fmt.Errorf("unrecognised statement format")
	}
}

// Parse detects the format of r and parses it into transactions.
func Parse(r io.ReadSeeker) ([]txns.Transaction, error) {
	format, err := Detect(r)
	if err != nil {
		return nil, err
	}
	switch format {
	case FormatDBSAccount:
		return ParseDBSAccount(r)
	case FormatDBSCard:
		return ParseDBSCard(r)
	case FormatUOBCard:
		return ParseUOBCard(r)
	default:
		return nil, fmt.Errorf("unrecognised statement format")
	}
}

// parseAmt parses a statement amount, tolerating thousands separators and blank
// cells (which mean zero).
func parseAmt(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

// collapse trims and squeezes internal whitespace (including newlines) to single
// spaces.
func collapse(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

// cell returns the trimmed value of column i in a CSV record, or "" if the
// record is too short.
func cell(rec []string, i int) string {
	if i < len(rec) {
		return strings.TrimSpace(rec[i])
	}
	return ""
}
