package txns

import "testing"

func TestRebind(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no placeholders", "SELECT 1", "SELECT 1"},
		{"numbers in order", "WHERE a = ? AND b = ?", "WHERE a = $1 AND b = $2"},
		{"past ten", "VALUES (?,?,?,?,?,?,?,?,?,?,?)", "VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)"},
		{
			"the shape handlers actually build",
			"SELECT * FROM transactions WHERE date >= ? AND owner = ? ORDER BY date DESC LIMIT ? OFFSET ?",
			"SELECT * FROM transactions WHERE date >= $1 AND owner = $2 ORDER BY date DESC LIMIT $3 OFFSET $4",
		},
		{
			"a ? inside a string literal is not a placeholder",
			"SELECT ? WHERE reference LIKE '%what?%' AND a = ?",
			"SELECT $1 WHERE reference LIKE '%what?%' AND a = $2",
		},
		{
			"a doubled quote does not end the literal early",
			"SELECT 'it''s a ? here', ?",
			"SELECT 'it''s a ? here', $1",
		},
		{
			"a ? inside a quoted identifier is not a placeholder",
			`SELECT "odd?column" FROM t WHERE a = ?`,
			`SELECT "odd?column" FROM t WHERE a = $1`,
		},
		{
			"format strings survive untouched",
			"SELECT to_char(date, 'YYYY-MM') AS month FROM transactions WHERE date <= ?",
			"SELECT to_char(date, 'YYYY-MM') AS month FROM transactions WHERE date <= $1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rebind(c.in); got != c.want {
				t.Errorf("rebind(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	// A NUMERIC column arrives as its decimal string; the API has always handed
	// the UI a number, and num() only understands float64 and int64.
	if got := normalize("NUMERIC", "-509.2500"); got != -509.25 {
		t.Errorf("NUMERIC: got %v (%T), want -509.25", got, got)
	}
	if got := normalize("NUMERIC", []byte("12.5")); got != 12.5 {
		t.Errorf("NUMERIC bytes: got %v (%T), want 12.5", got, got)
	}
	// NULL stays NULL rather than becoming a zero amount.
	if got := normalize("NUMERIC", nil); got != nil {
		t.Errorf("NUMERIC null: got %v, want nil", got)
	}
	if got := normalize("DATE", nil); got != nil {
		t.Errorf("DATE null: got %v, want nil", got)
	}
	// TEXT is left alone, including something that looks numeric.
	if got := normalize("TEXT", "2026-07"); got != "2026-07" {
		t.Errorf("TEXT: got %v, want 2026-07", got)
	}
	if got := normalize("INT8", int64(1770)); got != int64(1770) {
		t.Errorf("INT8: got %v (%T), want int64 1770", got, got)
	}
}

// A statement writes the account number or card PAN after the product name, and
// those identifiers are deliberately not in the source tree, so a label is
// recognised by its leading product name and whatever follows is ignored.
func TestShortName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// The identifiers here are invented: the point of matching on the prefix
		// is that the real ones need never appear, in the source or in a test.
		{"account label carrying an account number", "Personal Main 000-000000-0", "Personal Main"},
		{"the other account", "Joint Budgeting 111-111111-1", "Joint Budgeting"},
		{"card label carrying a PAN", "DBS MasterCard World 0000-0000-0000-0000", "DBS MCW"},
		{"a different account number maps the same way", "Personal Main 222-222222-2", "Personal Main"},
		{"label with no trailing identifier at all", "Joint Budgeting", "Joint Budgeting"},
		{"UOB label, which carries no number", "UOB PRVI MILES VISA CARD", "UOB PRVI MILES"},
		{"an unknown account is passed through unchanged", "Some Other Bank 123", "Some Other Bank 123"},
		{"matching is anchored at the start, not anywhere", "Transfer to Personal Main", "Transfer to Personal Main"},
	}

	for _, c := range cases {
		if got := ShortName(c.in); got != c.want {
			t.Errorf("%s: ShortName(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
