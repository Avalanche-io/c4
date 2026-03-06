package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
)

// runUnrm implements "c4 unrm" — list or recover items removed from a managed directory.
//
//	c4 unrm :                   # list recoverable items by snapshot
//	c4 unrm :~1/draft.exr      # recover a specific item
func runUnrm(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  c4 unrm :              # list recoverable items\n")
		fmt.Fprintf(os.Stderr, "  c4 unrm :~N/path       # recover item from snapshot N\n")
		os.Exit(1)
	}

	target := args[0]

	d, err := managed.Open(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if target == ":" {
		// List recoverable items: entries that exist in prior snapshots but not current
		listRecoverable(d)
		return
	}

	// Recovery: :~N/path
	if !strings.HasPrefix(target, ":~") {
		fmt.Fprintf(os.Stderr, "Error: target must be : or :~N/path\n")
		os.Exit(1)
	}

	ref := strings.TrimPrefix(target, ":~")
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		fmt.Fprintf(os.Stderr, "Error: specify path to recover (e.g. :~1/file.txt)\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Recovery not yet implemented (requires filesystem materialization)\n")
	os.Exit(1)
}

// listRecoverable shows items that exist in prior snapshots but not in current.
func listRecoverable(d *managed.Dir) {
	current, err := d.Current()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Build set of current entry names (with full paths)
	currentNames := entryNames(current)

	history, err := d.History()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check each prior snapshot for entries not in current
	found := false
	for _, h := range history {
		if h.Index == 0 {
			continue // skip current
		}
		snap, err := d.GetSnapshot(h.Index)
		if err != nil {
			continue
		}
		snapNames := entryNames(snap)

		var removed []string
		for name := range snapNames {
			if !currentNames[name] {
				removed = append(removed, name)
			}
		}

		if len(removed) > 0 {
			if found {
				fmt.Println()
			}
			found = true
			fmt.Printf("snapshot ~%d:\n", h.Index)
			for _, name := range removed {
				fmt.Printf("  %s\n", name)
			}
		}
	}

	if !found {
		fmt.Println("no recoverable items")
	}
}

// entryNames returns a set of full path names from a manifest.
func entryNames(m *c4m.Manifest) map[string]bool {
	names := make(map[string]bool)
	var dirStack []string
	for _, e := range m.Entries {
		if e.Depth < len(dirStack) {
			dirStack = dirStack[:e.Depth]
		}
		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + e.Name
		} else {
			fullPath = e.Name
		}
		names[fullPath] = true
		if e.IsDir() {
			for len(dirStack) <= e.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[e.Depth] = e.Name
		}
	}
	return names
}
