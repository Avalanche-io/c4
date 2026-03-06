package main

import (
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
)

// runUndo implements "c4 undo :" — revert the last operation on a managed directory.
func runUndo(args []string) {
	if len(args) != 1 || args[0] != ":" {
		fmt.Fprintf(os.Stderr, "Usage: c4 undo :    # revert last operation\n")
		os.Exit(1)
	}

	d, err := managed.Open(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := d.Undo(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m, _ := d.Current()
	history, _ := d.History()
	fmt.Printf("undone — now at snapshot ~0 (%d entries, %d in history)\n", len(m.Entries), len(history))
}

// runRedo implements "c4 redo :" — re-apply the last undone operation.
func runRedo(args []string) {
	if len(args) != 1 || args[0] != ":" {
		fmt.Fprintf(os.Stderr, "Usage: c4 redo :    # re-apply undone operation\n")
		os.Exit(1)
	}

	d, err := managed.Open(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := d.Redo(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m, _ := d.Current()
	history, _ := d.History()
	fmt.Printf("redone — now at snapshot ~0 (%d entries, %d in history)\n", len(m.Entries), len(history))
}
