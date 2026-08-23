// Command dummyseed fills the demo household — Chiikawa and Hachiware — with a
// year of invented transactions and the owner rules that sort the personal ones
// out of the joint budget. It is what makes a public deployment show the app
// working without anybody's bank statements in it.
//
//	DEMO_MODE=true go run ./cmd/dummyseed
//
// It refuses to run outside demo mode. Everything it writes is stamped is_dummy
// by txns.Insert from that same flag, so the guard is not belt and braces: it is
// the only thing standing between a seeding run and a thousand fake rows landing
// in the real household. For the same reason the clear-out at the start goes
// through txns.Dataset, which in demo mode is `is_dummy IS TRUE` and can reach
// nothing else.
//
// Seeding is idempotent: it clears the demo dataset and rebuilds it from a fixed
// random seed, so two runs leave the same household behind rather than two of
// them. The window ends today and runs back twelve months, which is the span the
// dashboard averages a recurring bill over (recurringWindow in main.go), so the
// demo lands on a dashboard with every panel populated.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"finances/internal/txns"
)

// The two partners, as budget_contributors already names them. A transaction's
// owner is a foreign key to that table, so these strings have to match the rows
// seeded there by hand.
const (
	chiikawa  = "Chiikawa"
	hachiware = "Hachiware"
)

// The accounts, using the canonical short names txns.ShortName maps statements
// onto — the demo is meant to look like an import, not like something typed in.
const (
	budgetAccount = "Joint Budgeting" // the pooled account both partners pay into
	cardAccount   = "DBS MCW"         // the everyday joint credit card
	milesCard     = "UOB PRVI MILES"  // the travel card
	personal      = "Personal Main"   // where salaries land and personal bills leave from
)

// months is how far back the window reaches, and matches the dashboard's
// recurringWindow so a monthly bill has a full history to be averaged over.
const months = 12

func main() {
	dry := flag.Bool("n", false, "generate and summarise, but write nothing")
	flag.Parse()

	if err := txns.Open(); err != nil {
		fail(err)
	}
	defer txns.Close()

	if !txns.Demo() {
		fail(fmt.Errorf("refusing to run outside demo mode: set DEMO_MODE=true, or this would write invented transactions into the real household"))
	}

	end := time.Now()
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)

	g := &generator{rnd: rand.New(rand.NewSource(20260823)), start: start, end: end}
	rows := g.household()

	if *dry {
		fmt.Printf("%d transactions from %s to %s (not written)\n", len(rows), start.Format("2006-01-02"), end.Format("2006-01-02"))
		summarise(rows)
		return
	}

	// Clear first, so a second run replaces the demo household rather than
	// doubling it. Both statements are scoped to the demo dataset.
	cleared, err := txns.Exec("DELETE FROM transactions WHERE " + txns.Dataset(""))
	if err != nil {
		fail(err)
	}
	n, _ := cleared.RowsAffected()
	fmt.Printf("cleared %d demo transactions\n", n)

	cleared, err = txns.Exec("DELETE FROM owner_rules WHERE " + txns.Dataset(""))
	if err != nil {
		fail(err)
	}
	n, _ = cleared.RowsAffected()
	fmt.Printf("cleared %d demo owner rules\n", n)

	if err := insertOwnerRules(); err != nil {
		fail(err)
	}
	fmt.Printf("inserted %d owner rules\n", len(ownerRules))

	// The rules go in before the transactions so AssignOwners can read them back
	// and tag the personal rows the same way an import would — by matching the
	// reference, not by the generator asserting an owner it already knew. Rows
	// this seeder assigned by hand (see manual, below) already carry an owner and
	// are left alone, which is exactly how a deliberate override behaves.
	tagged, err := txns.AssignOwners(rows)
	if err != nil {
		fail(err)
	}
	fmt.Printf("owner rules tagged %d transactions\n", tagged)

	written, err := txns.Insert(rows)
	if err != nil {
		fail(err)
	}
	fmt.Printf("inserted %d transactions from %s to %s\n", written, start.Format("2006-01-02"), end.Format("2006-01-02"))
	summarise(rows)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// summarise prints what the run produced, so the shape of the demo household is
// visible without querying it back.
func summarise(rows []txns.Transaction) {
	var income, spend float64
	owned := map[string]int{}
	for _, t := range rows {
		if t.Amount > 0 {
			income += t.Amount
		} else {
			spend += t.Amount
		}
		if t.Owner != nil {
			owned[*t.Owner]++
		}
	}
	fmt.Printf("  credits %.2f, spend %.2f\n", income, spend)
	for _, name := range []string{chiikawa, hachiware} {
		fmt.Printf("  %s: %d personal transactions\n", name, owned[name])
	}
}

// ownerRules is the demo household's owner_rules, keyed to the references the
// generator writes below. They cover the three kinds of personal money the app
// exists to keep out of the joint budget: a partner's own salary, their own
// insurance, and the allowance they send their parents.
//
// The insurers are pinned to a category on purpose, because the demo should show
// why pinning exists: USAGI ASSURANCE bills Chiikawa for her policy every month,
// and the same insurer pays the household out on a claim. The premium is
// personal, the payout is joint, and only the category tells them apart. MOMONGA
// GYM is left unpinned as the other half of the contrast — wherever that name
// turns up, it is Hachiware's membership.
var ownerRules = []struct {
	keyword  string
	category string // "" = applies in any category
	owner    string
}{
	{"SALARY BY :KUSA CORP", "💰 Income", chiikawa},
	{"USAGI ASSURANCE", "🛡️ Insurance", chiikawa},
	{"REF:CHIIKAWA ALLOWANCE", "🎁 Gifts & Donations", chiikawa},
	{"IRAS ITX S1904772C", "💸 Income Tax", chiikawa},
	{"SALARY BY :RAMEN GALLERY", "💰 Income", hachiware},
	{"SHISA HEALTH", "🛡️ Insurance", hachiware},
	{"REF:HACHIWARE ALLOWANCE", "🎁 Gifts & Donations", hachiware},
	{"IRAS ITX S2288451J", "💸 Income Tax", hachiware},
	{"MOMONGA GYM", "", hachiware},
}

func insertOwnerRules() error {
	// Built inline rather than prepared, for the reason the package documents:
	// the pooler runs in transaction mode and no named statement survives it.
	values := make([]string, 0, len(ownerRules))
	args := make([]any, 0, len(ownerRules)*3)
	n := 0
	for _, r := range ownerRules {
		var category any
		if r.category != "" {
			category = r.category
		}
		values = append(values, fmt.Sprintf("($%d, $%d, $%d, TRUE)", n+1, n+2, n+3))
		args = append(args, r.keyword, category, r.owner)
		n += 3
	}
	_, err := txns.Exec(
		"INSERT INTO owner_rules (keyword, category, owner, is_dummy) VALUES "+strings.Join(values, ", "),
		args...,
	)
	return err
}
