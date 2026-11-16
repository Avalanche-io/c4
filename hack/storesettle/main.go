// Command storesettle checks a local tree store for objects sitting above
// their proper level, and moves them down with -move. Processes sharing one
// store before 2026-11-16 could leave such files when one split a directory
// while another was writing (the store split race). Lookups from this
// release on find them where they are, so a store reads correctly without
// this; moving them is for older readers sharing the store.
//
//	go run ./hack/storesettle DIR          report only (the default)
//	go run ./hack/storesettle -move DIR    move each one to its proper level
//
// DIR is the store's root: C4_STORE when set, else ~/.c4/store. stdout lists
// one file per line, "misplaced", "moved" or "duplicate" (the proper place
// already holds the object; nothing is removed), then its path and the path
// the trie puts it at. A summary goes to stderr. It exits 0 when nothing is
// left above its level, 2 when something is (found in a report, or a
// duplicate), and 1 on an error.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/store"
)

func main() {
	move := flag.Bool("move", false, "move each misplaced object to its proper level")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: storesettle [-move] DIR")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	os.Exit(run(flag.Arg(0), *move))
}

func run(dir string, move bool) int {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "storesettle: %s is not a store directory\n", dir)
		return 1
	}
	s, err := store.NewTreeStore(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "storesettle: %v\n", err)
		return 1
	}
	report, err := s.Settle(move)
	if err != nil {
		fmt.Fprintf(os.Stderr, "storesettle: %v\n", err)
		return 1
	}
	left := 0
	for _, m := range report {
		state := "misplaced"
		switch {
		case m.Moved:
			state = "moved"
		case move:
			state = "duplicate"
			left++
		default:
			left++
		}
		fmt.Printf("%s %s %s\n", state, m.Path, m.Want)
	}
	fmt.Fprintf(os.Stderr, "storesettle: %d object files above their level, %d moved, %d left\n", len(report), len(report)-left, left)
	if left > 0 {
		return 2
	}
	return 0
}
