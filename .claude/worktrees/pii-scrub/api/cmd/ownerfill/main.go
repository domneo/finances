// Command ownerfill replays owner_rules over every transaction already in the
// database, tagging the ones that belong to a partner's personal budget and
// clearing those that no longer match a rule. Owners set by hand are left as
// they are. Run it after editing owner_rules.
//
//	go run ./cmd/ownerfill
package main

import (
	"fmt"
	"os"

	"finances/internal/txns"
)

func main() {
	if err := txns.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer txns.Close()

	n, protected, err := txns.ApplyOwnerRules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "apply: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("updated %d transactions\n", n)
	if protected > 0 {
		fmt.Printf("left %d manually assigned transactions untouched\n", protected)
	}
}
