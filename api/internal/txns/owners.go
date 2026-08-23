package txns

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrUnknownOwner is returned when a write names an owner who is not a budget
// contributor. Both owner columns are foreign keys to budget_contributors.name,
// so the database refuses the row rather than storing a partner nobody budgets
// for; without this the refusal would reach the client as a 500 carrying the
// constraint's name, when it is really a bad request.
var ErrUnknownOwner = errors.New("unknown owner: not a budget contributor")

// ownerFKError converts a foreign-key violation on one of the owner columns
// into ErrUnknownOwner and leaves every other error alone. The category columns
// are foreign keys too, so the constraint name is checked rather than the
// SQLSTATE alone — a missing category is a different mistake.
func ownerFKError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && strings.HasSuffix(pgErr.ConstraintName, "_owner_fkey") {
		return ErrUnknownOwner
	}
	return err
}

// Most transactions are joint: the household pays them and they belong on the
// shared budget. A few kinds are personal — one partner's own insurance, salary
// or allowance to their parents — and are budgeted for separately. The owner
// column names that partner; NULL means the transaction is joint.
//
// Which transactions are personal is decided by owner_rules rather than by
// hand, so an import tags itself and a rule change can be replayed over the
// history. A row can still be assigned by hand, though, and the owner_source
// column says which happened: a rule's own work is fair game for the next
// replay, a person's decision is not.

// ownerRule is one row of owner_rules: a substring to look for in a
// transaction's reference, the partner it belongs to, and optionally the one
// category it applies in.
type ownerRule struct {
	keyword  string
	category string // "" = applies in any category
	owner    string
}

// matches reports whether a transaction falls under the rule.
func (r ownerRule) matches(reference, category string) bool {
	if !strings.Contains(strings.ToUpper(reference), strings.ToUpper(r.keyword)) {
		return false
	}
	return r.category == "" || r.category == category
}

// loadOwnerRules reads owner_rules in match order. A rule pinned to a category
// is tried before an unpinned one — the same counterparty can run both ways,
// and only one side is personal: an insurer's premiums are one partner's own
// insurance while its payouts are a joint loan. Among equally pinned rules the
// longest (most specific) keyword wins.
//
// An unpinned rule stores the empty string for its category in Postgres, where
// it was NULL under SQLite, so the ordering coalesces before comparing instead
// of leaning on where NULLs happen to sort.
func loadOwnerRules() ([]ownerRule, error) {
	rows, err := QueryRows(
		`SELECT keyword, COALESCE(category, '') AS category, owner FROM owner_rules
		 WHERE ` + Dataset("") + `
		 ORDER BY COALESCE(category, '') = '', LENGTH(keyword) DESC`,
	)
	if err != nil {
		return nil, err
	}

	rules := make([]ownerRule, 0, len(rows))
	for _, row := range rows {
		keyword, _ := row["keyword"].(string)
		category, _ := row["category"].(string)
		owner, _ := row["owner"].(string)
		if keyword == "" || owner == "" {
			continue
		}
		rules = append(rules, ownerRule{keyword: keyword, category: category, owner: owner})
	}
	return rules, nil
}

// ownerFor returns the partner the rules give a transaction, or "" if no rule
// claims it — in which case the transaction is joint. Rules are tried in the
// order loadOwnerRules returned them, so the first match is the most specific.
func ownerFor(rules []ownerRule, reference, category string) string {
	for _, rule := range rules {
		if rule.matches(reference, category) {
			return rule.owner
		}
	}
	return ""
}

// AssignOwners tags every unowned transaction that matches an owner rule,
// mutating rows in place; the rest stay joint. Rules can be pinned to a
// category, so run this after CategorizeRows — a row still missing its category
// cannot match a pinned rule. Returns the number of rows tagged.
func AssignOwners(rows []Transaction) (int, error) {
	rules, err := loadOwnerRules()
	if err != nil {
		return 0, err
	}

	n := 0
	for i := range rows {
		if rows[i].Owner != nil || rows[i].Reference == nil {
			continue
		}
		category := ""
		if rows[i].Category != nil {
			category = *rows[i].Category
		}
		owner := ownerFor(rules, *rows[i].Reference, category)
		if owner == "" {
			continue
		}
		source := "rule"
		rows[i].Owner = &owner
		rows[i].OwnerSource = &source
		n++
	}
	return n, nil
}

// ResolveOwnerSources records who decided each row's owner, so a later replay
// knows what it is allowed to overwrite. It compares the owner on the row with
// the rules' verdict: where the two agree — including agreeing that the row is
// joint — the rules own the decision and may revise it later; any divergence is
// somebody's deliberate override and is marked manual, which puts it beyond the
// reach of ApplyOwnerRules until a person changes it again. Rows arrive from
// clients, so whatever OwnerSource they carry is overwritten rather than
// trusted. Call this on rows headed for Insert.
func ResolveOwnerSources(rows []Transaction) error {
	rules, err := loadOwnerRules()
	if err != nil {
		return err
	}

	for i := range rows {
		reference, category, owner := "", "", ""
		if rows[i].Reference != nil {
			reference = *rows[i].Reference
		}
		if rows[i].Category != nil {
			category = *rows[i].Category
		}
		if rows[i].Owner != nil {
			owner = *rows[i].Owner
		}

		var source string
		switch verdict := ownerFor(rules, reference, category); {
		case owner != verdict:
			source = "manual"
		case verdict != "":
			source = "rule"
		default:
			rows[i].OwnerSource = nil
			continue
		}
		rows[i].OwnerSource = &source
	}
	return nil
}

// ApplyOwnerRules replays the rules over transactions already in the database.
// Every rule-assigned row is reconsidered, so removing or re-pointing a rule
// takes effect on the history too, which makes this safe to re-run after any
// edit to owner_rules. Rows a person assigned by hand are left alone — that is
// what makes a one-off override survive the next replay. Returns the number of
// rows whose owner changed and the number of manual rows the rules wanted to
// change but were not allowed to.
func ApplyOwnerRules() (changed, protected int, err error) {
	rules, err := loadOwnerRules()
	if err != nil {
		return 0, 0, err
	}

	rows, err := QueryRows(
		`SELECT id, COALESCE(reference, '') AS reference,
		        COALESCE(category, '') AS category,
		        owner, COALESCE(owner_source, '') AS owner_source
		 FROM transactions WHERE ` + Dataset(""),
	)
	if err != nil {
		return 0, 0, err
	}

	tx, err := DB.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	for _, row := range rows {
		reference, _ := row["reference"].(string)
		category, _ := row["category"].(string)
		current, _ := row["owner"].(string) // "" for NULL
		source, _ := row["owner_source"].(string)

		owner := ownerFor(rules, reference, category)
		if owner == current {
			continue
		}
		if source == "manual" {
			protected++
			continue
		}

		var value, newSource any
		if owner != "" {
			value, newSource = owner, "rule"
		}
		if _, err := tx.Exec(
			"UPDATE transactions SET owner = $1, owner_source = $2 WHERE id = $3",
			value, newSource, row["id"],
		); err != nil {
			return 0, 0, err
		}
		changed++
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return changed, protected, nil
}

// SetOwner hands a single transaction to a partner, or back to the joint budget
// when owner is nil or empty. The decision is judged against the rules the same
// way ResolveOwnerSources judges an import: choosing what they would have chosen
// leaves the row theirs to revise later, anything else is a manual override that
// outlives the next replay. Reports whether the transaction exists.
func SetOwner(id int, owner *string) (bool, error) {
	row, err := QueryOne(
		`SELECT COALESCE(reference, '') AS reference, COALESCE(category, '') AS category
		 FROM transactions WHERE id = ? AND `+Dataset(""), id,
	)
	if err != nil || row == nil {
		return false, err
	}

	rules, err := loadOwnerRules()
	if err != nil {
		return false, err
	}
	reference, _ := row["reference"].(string)
	category, _ := row["category"].(string)

	value := ""
	if owner != nil {
		value = strings.TrimSpace(*owner)
	}

	var ownerArg, sourceArg any
	if value != "" {
		ownerArg = value
	}
	switch verdict := ownerFor(rules, reference, category); {
	case value != verdict:
		sourceArg = "manual"
	case verdict != "":
		sourceArg = "rule"
	}

	if _, err := Exec(
		"UPDATE transactions SET owner = ?, owner_source = ? WHERE id = ?",
		ownerArg, sourceArg, id,
	); err != nil {
		return false, ownerFKError(err)
	}
	return true, nil
}

// Owners lists every partner who can hold a personal budget, so the UI can
// offer them before any matching transaction exists.
//
// It reads budget_contributors because that table is now what defines a
// partner: transactions.owner and owner_rules.owner are foreign keys to
// budget_contributors.name, so no other name can reach either column. The list
// used to be gathered from the two owner columns instead, which could only name
// a partner some row already pointed at — a contributor added before their
// first rule or transaction was missing from the dropdown that was supposed to
// let somebody give them one.
func Owners() ([]string, error) {
	rows, err := QueryRows(
		`SELECT name AS owner FROM budget_contributors WHERE ` + Dataset("") + ` ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	owners := make([]string, 0, len(rows))
	for _, row := range rows {
		if v, ok := row["owner"].(string); ok {
			owners = append(owners, v)
		}
	}
	return owners, nil
}
