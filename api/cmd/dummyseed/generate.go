package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"finances/internal/txns"
)

// The generator builds the demo household month by month, in the shape a real
// year of statements has: a handful of fixed bills that land on the same day
// every month, a few yearly ones that land once, a long tail of card spending
// that varies, and the personal money — salary in, insurance and allowances out
// — that the owner rules exist to lift out of the joint budget.
//
// Nothing here is random enough to surprise: the seed is fixed in main, so the
// same household comes back every run, and the amounts jitter around a base
// rather than being drawn freely. A demo that reshuffles itself on every deploy
// is a demo nobody can screenshot.
type generator struct {
	rnd   *rand.Rand
	start time.Time // first day of the earliest month in the window
	end   time.Time // today; the current month is partial, as a real one is
}

// household assembles the whole dataset.
func (g *generator) household() []txns.Transaction {
	var rows []txns.Transaction
	for m := g.start; !m.After(g.end); m = m.AddDate(0, 1, 0) {
		rows = append(rows, g.contributions(m)...)
		rows = append(rows, g.fixedBills(m)...)
		rows = append(rows, g.yearlyBills(m)...)
		rows = append(rows, g.variableSpend(m)...)
		rows = append(rows, g.personal(m)...)
		rows = append(rows, g.oneOffs(m)...)
	}
	return rows
}

// tx builds one transaction, dropping it if the date has not happened yet — the
// current month is part-way through, so a bill due on the 25th of it should not
// already be in the statement.
func (g *generator) tx(date time.Time, account, category, reference string, amount float64) []txns.Transaction {
	if date.After(g.end) || date.Before(g.start) {
		return nil
	}
	return []txns.Transaction{{
		Date:      date.Format("2006-01-02"),
		Account:   account,
		Category:  &category,
		Reference: &reference,
		Amount:    round(amount),
		Currency:  "SGD",
	}}
}

// manual marks rows as assigned by a person rather than by a rule, which is
// what owner_source exists to record and what ApplyOwnerRules refuses to
// overwrite. The demo needs a couple, or that half of the feature is invisible.
func manual(rows []txns.Transaction, owner string) []txns.Transaction {
	source := "manual"
	for i := range rows {
		rows[i].Owner = &owner
		rows[i].OwnerSource = &source
	}
	return rows
}

// on returns a day of the given month, clamped to its last day so the 31st
// still lands in February.
func on(month time.Time, day int) time.Time {
	last := month.AddDate(0, 1, -1).Day()
	return time.Date(month.Year(), month.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
}

func round(v float64) float64 { return float64(int64(v*100+sign(v)*0.5)) / 100 }

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// jitter varies an amount around a base by up to spread either way, so a
// utilities bill is not the same number twelve times over.
func (g *generator) jitter(base, spread float64) float64 {
	if spread == 0 {
		return base
	}
	return base + (g.rnd.Float64()*2-1)*spread
}

// between draws an amount in a range, for spending that has no fixed size.
func (g *generator) between(lo, hi float64) float64 { return lo + g.rnd.Float64()*(hi-lo) }

// day picks a day of the month, weighted to nothing in particular.
func (g *generator) day(month time.Time) time.Time {
	return on(month, 1+g.rnd.Intn(month.AddDate(0, 1, -1).Day()))
}

// ref numbers give statement lines the run of digits a bank puts on them, so
// the demo's references read like an import rather than like a fixture.
func (g *generator) digits(n int) string {
	s := ""
	for range n {
		s += string(rune('0' + g.rnd.Intn(10)))
	}
	return s
}

// contributions are the standing instructions that fund the joint account: the
// credit the dashboard's budget panel looks for, and the matching debit out of
// the payer's own account. The reference has to contain the payer's name as
// budget_contributors.reference_match spells it, which is what makes the panel
// recognise it.
func (g *generator) contributions(month time.Time) []txns.Transaction {
	var rows []txns.Transaction
	for _, c := range []struct {
		name   string
		amount float64
	}{{chiikawa, 1000}, {hachiware, 5000}} {
		date := on(month, 1)
		stamp := date.Format("060102") + "0751" + g.digits(2) + "SI" + g.digits(8)
		rows = append(rows, g.tx(date, budgetAccount, "🔄 Transfer",
			fmt.Sprintf("SI BY :%s PART/REF:BUDGET %s", strings.ToUpper(c.name), stamp), c.amount)...)
		rows = append(rows, g.tx(date, personal, "🔄 Transfer",
			fmt.Sprintf("SI TO :JOINT BUDGET REF:%s %s", strings.ToUpper(c.name), stamp), -c.amount)...)
	}
	return rows
}

// fixedBills are the monthly recurring charges the dashboard averages over the
// window and then checks off against this month. Everything on a GIRO or a
// standing instruction leaves the joint account; the subscriptions sit on the
// everyday card, as they do in the real household.
func (g *generator) fixedBills(month time.Time) []txns.Transaction {
	bills := []struct {
		day          int
		account      string
		category     string
		reference    string
		base, spread float64
	}{
		{1, budgetAccount, "🅿 Season Parking", "GIRO PAYMENT TO KUSAMURA CARPARK MGT", -110, 0},
		{2, cardAccount, "🧘‍♀️ Classpass", "CLASSPASS SINGAPORE SGP", -99, 0},
		{3, budgetAccount, "📦 Google Storage", "GOOGLE *GOOGLE STORAGE SINGAPORE SGP", -2.79, 0},
		{5, budgetAccount, "🏠 S&C Charges", "GIRO PAYMENT TO MOGURA TOWN COUNCIL", -318.60, 0},
		{6, cardAccount, "🎧 Spotify", "SPOTIFY SINGAPORE SGP", -17.98, 0},
		{8, budgetAccount, "🏠 Utilities bill", "GIRO PAYMENT TO SP SERVICES REF 8801-2344", -178, 46},
		{10, budgetAccount, "🚗 Car Loan", "GIRO PAYMENT TO USAGI MOTOR CREDIT", -688, 0},
		{10, budgetAccount, "🚗 Car Insurance", "GIRO PAYMENT TO NAMAKO GENERAL INSURANCE", -142.50, 0},
		{12, budgetAccount, "📡 Wifi bill", "GIRO PAYMENT TO HACHINET BROADBAND", -49.90, 0},
		{15, budgetAccount, "📱 Phone Bill", "GIRO PAYMENT TO KURIMANJU MOBILE 9123", -28.90, 0},
		{15, budgetAccount, "📱 Phone Bill", "GIRO PAYMENT TO KURIMANJU MOBILE 9455", -32.90, 0},
		{16, cardAccount, "📺 Netflix", "NETFLIX.COM LOS GATOS SGP", -22.98, 0},
		{22, cardAccount, "📺 Amazon Prime", "AMAZON PRIME SG SINGAPORE SGP", -2.99, 0},
		{28, budgetAccount, "🏦 Interest", "Interest Earned", 2.4, 1.1},
	}

	var rows []txns.Transaction
	for _, b := range bills {
		rows = append(rows, g.tx(on(month, b.day), b.account, b.category, b.reference, g.jitter(b.base, b.spread))...)
	}
	return rows
}

// yearlyBills land once a year each, spread across the calendar so the demo's
// recurring panel has yearly rows in every state: one charged this month, some
// charged months ago, one still to come.
func (g *generator) yearlyBills(month time.Time) []txns.Transaction {
	bills := []struct {
		month     time.Month
		day       int
		account   string
		category  string
		reference string
		amount    float64
	}{
		{time.January, 20, budgetAccount, "🏠 Property Tax", "GIRO IRAS PROPERTY TAX 1204776C", -1180},
		{time.February, 14, cardAccount, "🔑 1Password", "1PASSWORD TORONTO CAN", -95.88},
		{time.March, 9, cardAccount, "🏠 Home Insurance", "SHISA GENERAL HOME PLUS SINGAPORE SGP", -268.40},
		{time.April, 3, budgetAccount, "🚗 Road Tax", "ONEMOTORING ROAD TAX SINGAPORE SGP", -742},
		{time.June, 26, cardAccount, "🐱 Pet Insurance", "MOMONGA PET COVER SINGAPORE SGP", -412},
		{time.November, 11, budgetAccount, "🏠 Fire Insurance", "GIRO PAYMENT TO NAMAKO GENERAL INSURANCE FIRE", -78},
	}

	var rows []txns.Transaction
	for _, b := range bills {
		if month.Month() != b.month {
			continue
		}
		rows = append(rows, g.tx(on(month, b.day), b.account, b.category, b.reference, b.amount)...)
	}
	return rows
}

// merchants is the card spending the household actually does: how many times a
// month, for how much, and under whose name. Dining dominates the count and
// groceries the value, which is what makes the variable panel look like a real
// month rather than an even spread.
var merchants = []struct {
	category string
	account  string
	lo, hi   int     // transactions per month
	min, max float64 // spend per transaction, before the sign
	names    []string
}{
	{"🍴 Dining", cardAccount, 14, 22, 5, 68, []string{
		"KURIMANJU COFFEE SINGAPORE SGP", "RAMEN GALLERY SINGAPORE SGP", "TOAST & KAYA CAFE SINGAPORE SGP",
		"HACHIWARE BAKERY SINGAPORE SGP", "MOGU MOGU HOTPOT SINGAPORE SGP", "SHISA CHICKEN RICE SINGAPORE SGP",
		"YOMOGI TEA HOUSE SINGAPORE SGP", "USAGI IZAKAYA SINGAPORE SGP", "KUSA SALAD BAR SINGAPORE SGP",
	}},
	{"🛒 Groceries", cardAccount, 6, 10, 14, 165, []string{
		"NTUC FAIRPRICE APP PAY SINGAPORE 065", "COLD STORAGE TIONG BAHRU SGP",
		"SHENG SIONG SUPERMARKET SGP", "MOGURA MARKET SINGAPORE SGP", "REDMART SINGAPORE SGP",
	}},
	{"🚆 Transportation", cardAccount, 8, 14, 1.4, 26, []string{
		"BUS/MRT %s SINGAPORE SGP", "GRAB RIDES-EC SINGAPORE SGP", "TADA MOBILITY SINGAPORE SGP",
	}},
	{"🅿 Parking & ERP", cardAccount, 3, 7, 1.2, 14, []string{
		"ERP CHARGES SINGAPORE SGP", "MOGURA MALL CARPARK SGP", "HDB CARPARK SINGAPORE SGP",
	}},
	{"⛽ Gas & Fuel", cardAccount, 2, 3, 58, 96, []string{
		"SHELL KUSAMURA SINGAPORE SGP", "ESSO HACHI ROAD SINGAPORE SGP", "SPC YOMOGI AVE SINGAPORE SGP",
	}},
	{"🐱 Pets", cardAccount, 1, 3, 22, 185, []string{
		"SP PETCUBES SINGAPORE SGP", "MOMONGA VET CLINIC SINGAPORE SGP", "PET LOVERS CENTRE SGP",
	}},
	{"🩲 Shopping", cardAccount, 2, 5, 18, 240, []string{
		"UNIQLO SINGAPORE SGP", "MUJI SINGAPORE SGP", "SHOPEE SINGAPORE SGP", "TAOBAO SINGAPORE SGP",
	}},
	{"🎮 Leisure", cardAccount, 1, 3, 12, 82, []string{
		"GOLDEN VILLAGE SINGAPORE SGP", "NINTENDO ESHOP SINGAPORE SGP", "KINOKUNIYA SINGAPORE SGP",
	}},
	{"💅 Personal Care", cardAccount, 1, 2, 24, 98, []string{
		"YOMOGI HAIR STUDIO SINGAPORE SGP", "WATSONS SINGAPORE SGP", "GUARDIAN SINGAPORE SGP",
	}},
	{"🧘‍♀️ Health & Fitness", cardAccount, 0, 2, 14, 62, []string{
		"YOGA MOVEMENT SINGAPORE SGP", "DECATHLON SINGAPORE SGP",
	}},
	{"👩‍🔧 Services", budgetAccount, 0, 2, 38, 180, []string{
		"ICT PayNow Transfer %s To: KUSA AIRCON SERVICING OTHR", "ICT PayNow Transfer %s To: MOGURA CLEANING PTE. LTD. OTHR",
	}},
	{"🎁 Gifts & Donations", cardAccount, 0, 2, 20, 130, []string{
		"GIVING.SG DONATION SINGAPORE SGP", "HACHIWARE FLORIST SINGAPORE SGP",
	}},
	{"💵 Cash", budgetAccount, 0, 1, 100, 200, []string{
		"ATM WITHDRAWAL MOGURA BRANCH SGP",
	}},
	{"💻 Electronics", milesCard, 0, 1, 45, 620, []string{
		"CHALLENGER SINGAPORE SGP", "APPLE STORE SINGAPORE SGP",
	}},
}

// variableSpend is the long tail: everything the household does not commit to
// in advance.
func (g *generator) variableSpend(month time.Time) []txns.Transaction {
	var rows []txns.Transaction
	for _, m := range merchants {
		n := m.lo
		if m.hi > m.lo {
			n += g.rnd.Intn(m.hi - m.lo + 1)
		}
		for range n {
			name := m.names[g.rnd.Intn(len(m.names))]
			if strings.Contains(name, "%s") {
				name = fmt.Sprintf(name, g.digits(7))
			}
			rows = append(rows, g.tx(g.day(month), m.account, m.category, name, -g.between(m.min, m.max))...)
		}
	}
	return rows
}

// personal is the money that belongs to one partner rather than the household:
// salary in, their own insurance and income tax out, and the allowance they send
// their parents. The generator does not set the owner — it writes the references
// the owner rules match on and lets AssignOwners do it, which is the same path
// an import takes and the only way to see whether the rules actually work.
func (g *generator) personal(month time.Time) []txns.Transaction {
	items := []struct {
		day       int
		category  string
		reference string
		amount    float64
	}{
		{5, "🎁 Gifts & Donations", "SI TO :PARENTS REF:CHIIKAWA ALLOWANCE " + g.digits(8), -400},
		{15, "💸 Income Tax", "GIRO IRAS ITX S1904772C", -213.40},
		{18, "🛡️ Insurance", "GIRO PAYMENT TO USAGI ASSURANCE PREMIUM 4471", -186.40},
		{25, "💰 Income", "SALARY BY :KUSA CORP PTE LTD REF:PAYROLL", 4280},
		{5, "🎁 Gifts & Donations", "SI TO :PARENTS REF:HACHIWARE ALLOWANCE " + g.digits(8), -600},
		{8, "🧘‍♀️ Health & Fitness", "MOMONGA GYM SINGAPORE SGP", -160},
		{15, "💸 Income Tax", "GIRO IRAS ITX S2288451J", -396.75},
		{20, "🛡️ Insurance", "GIRO PAYMENT TO SHISA HEALTH SHIELD 8820", -244.90},
		{25, "💰 Income", "SALARY BY :RAMEN GALLERY PTE LTD REF:PAYROLL", 8640},
	}

	var rows []txns.Transaction
	for _, it := range items {
		amount := it.amount
		if it.category == "💰 Income" {
			amount = g.jitter(amount, 60) // a variable component, as a real salary has
		}
		rows = append(rows, g.tx(on(month, it.day), personal, it.category, it.reference, amount)...)
	}
	return rows
}

// oneOffs are the things that happen once and would be missing from a household
// built only out of averages: two trips, an insurance payout, and a pair of
// transactions somebody claimed by hand.
func (g *generator) oneOffs(month time.Time) []txns.Transaction {
	var rows []txns.Transaction

	switch month.Month() {
	case time.December: // a week away over the year-end
		rows = append(rows, g.tx(on(month, 2), milesCard, "✈️ Travel", "SINGAPORE AIRLINES SINGAPORE SGP", -1848.60)...)
		rows = append(rows, g.tx(on(month, 19), milesCard, "✈️ Travel", "AGODA HOTEL BOOKING SINGAPORE SGP", -1132.00)...)
		rows = append(rows, g.tx(on(month, 22), milesCard, "🍴 Dining", "BAT KISSATEN NAKAMEGURO JPN 20DEC 4628-4500 JPY4200 "+g.digits(15), -36.40)...)
		rows = append(rows, g.tx(on(month, 24), milesCard, "🩲 Shopping", "BAT DONKI SHIBUYA JPN 22DEC 4628-4500 JPY18900 "+g.digits(15), -163.20)...)
	case time.June: // a long weekend up the causeway
		rows = append(rows, g.tx(on(month, 12), milesCard, "✈️ Travel", "KTMB GO TICKETING CYBERJAYA MY Ref No: "+g.digits(17), -186.40)...)
		rows = append(rows, g.tx(on(month, 13), milesCard, "🍴 Dining", "GRABFOOD PETALING JAYA MYS MYR 88.20", -26.10)...)
	case time.March:
		// The insurer that bills Chiikawa for her own policy also pays the
		// household out on a claim. Same counterparty, opposite side of the
		// ledger: the rule is pinned to 🛡️ Insurance, so this stays joint.
		rows = append(rows, g.tx(on(month, 17), budgetAccount, "🏦 Loan", "ICT USAGI ASSURANCE CLAIM PAYOUT REF "+g.digits(8), 1240)...)
	case time.May:
		// A joint-looking card charge that was really Hachiware's, claimed by
		// hand. No rule would ever produce this, and a replay must not undo it.
		rows = append(rows, manual(g.tx(on(month, 14), cardAccount, "💻 Electronics", "APPLE STORE SINGAPORE SGP", -1899), hachiware)...)
	case time.February:
		// And one the other way round: a card charge on the joint card that was
		// Chiikawa's own, claimed by hand under a merchant no rule names.
		rows = append(rows, manual(g.tx(on(month, 21), cardAccount, "🩲 Shopping", "MUJI SINGAPORE SGP", -318.20), chiikawa)...)
	}
	return rows
}
