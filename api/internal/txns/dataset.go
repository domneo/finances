package txns

import (
	"os"
	"strconv"
)

// This file holds the one switch that decides which of the two datasets in the
// database a process reads and writes.
//
// The three tables that hold personal data — transactions, owner_rules and
// budget_contributors — each carry an is_dummy flag, so a made-up household can
// sit beside the real one in the same Supabase project. That keeps a public
// deployment honest: it is the same schema, the same queries and the same code
// paths, with nobody's bank statements in it. The alternative, a second
// database, drifts out of shape the moment a column is added to only one of
// them.
//
// The flag is not a filter the caller may forget: every read of those tables
// goes through Dataset, and Insert stamps new rows with the mode that wrote
// them, so a statement imported in demo mode stays demo. Real is the default —
// an unset or unparseable DEMO_MODE serves the real household, because the way
// to get this wrong that matters is a demo deploy quietly showing real money,
// and that failure mode should need a deliberate act to reach.

// demoMode is whether this process serves the dummy dataset. It is set once by
// Open and read everywhere else, rather than threaded through every call, for
// the same reason DB is: it is a property of the process, fixed before the
// first query and never varying between requests.
var demoMode bool

// Demo reports whether this process serves the dummy dataset instead of the
// real one.
func Demo() bool { return demoMode }

// readDemoMode returns the value of DEMO_MODE from the environment, else from
// the dotenv file beside the binary, else false. Anything strconv.ParseBool
// rejects counts as false — see the note above on which way to fail.
func readDemoMode() bool {
	v := os.Getenv("DEMO_MODE")
	if v == "" {
		for _, path := range []string{".env", "api/.env"} {
			if v = fromEnvFile(path, "DEMO_MODE"); v != "" {
				break
			}
		}
	}
	on, err := strconv.ParseBool(v)
	return err == nil && on
}

// Dataset returns the SQL predicate confining a query to the dataset this
// process serves. qualifier is the table alias with its trailing dot ("t.")
// where the query uses one, and "" where it does not.
//
// It is written as IS TRUE / IS NOT TRUE rather than = true / = false because
// the column is nullable: a row inserted before the flag existed, or by hand in
// the Supabase editor, holds NULL, and = false would hide it from the real
// dataset it plainly belongs to.
func Dataset(qualifier string) string {
	if demoMode {
		return qualifier + "is_dummy IS TRUE"
	}
	return qualifier + "is_dummy IS NOT TRUE"
}
