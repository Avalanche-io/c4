package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

	snapRef := parts[0]
	itemPath := parts[1]

	// Load the snapshot
	var snap *c4m.Manifest
	if n, nerr := strconv.Atoi(snapRef); nerr == nil {
		snap, err = d.GetSnapshot(n)
	} else {
		snap, err = d.GetTag(snapRef)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	recoverFromSnapshot(snap, itemPath)
}

// recoverFromSnapshot finds an entry in a snapshot and writes it to the local filesystem.
func recoverFromSnapshot(snap *c4m.Manifest, itemPath string) {
	// Build full paths to find the entry
	type resolved struct {
		fullPath string
		entry    *c4m.Entry
	}
	var entries []resolved
	var dirStack []string

	for _, e := range snap.Entries {
		if e.Depth < len(dirStack) {
			dirStack = dirStack[:e.Depth]
		}
		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + e.Name
		} else {
			fullPath = e.Name
		}
		entries = append(entries, resolved{fullPath: fullPath, entry: e})
		if e.IsDir() {
			for len(dirStack) <= e.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[e.Depth] = e.Name
		}
	}

	// Find matching entry (exact match or directory prefix)
	var matched []resolved
	for _, r := range entries {
		if r.fullPath == itemPath {
			matched = append(matched, r)
		} else if strings.HasPrefix(r.fullPath, itemPath) {
			// Recovering a directory — include all children
			matched = append(matched, r)
		}
	}

	if len(matched) == 0 {
		fmt.Fprintf(os.Stderr, "Error: %s not found in snapshot\n", itemPath)
		os.Exit(1)
	}

	type dirMeta struct {
		path  string
		entry *c4m.Entry
	}
	var dirs []dirMeta

	recovered := 0
	for _, m := range matched {
		localPath := m.fullPath

		if m.entry.IsDir() {
			if err := os.MkdirAll(localPath, m.entry.Mode.Perm()); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", localPath, err)
				os.Exit(1)
			}
			dirs = append(dirs, dirMeta{localPath, m.entry})
			recovered++
			continue
		}

		if m.entry.IsSymlink() {
			dir := filepath.Dir(localPath)
			if dir != "." {
				os.MkdirAll(dir, 0755)
			}
			os.Remove(localPath)
			if err := os.Symlink(m.entry.Target, localPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating symlink %s: %v\n", localPath, err)
				os.Exit(1)
			}
			fmt.Printf("recovered %s → %s\n", localPath, m.entry.Target)
			recovered++
			continue
		}

		// Regular file — use writeFileContent (handles c4d fetch + exact metadata)
		dir := filepath.Dir(localPath)
		if dir != "." {
			os.MkdirAll(dir, 0755)
		}

		if err := writeFileContent(localPath, m.entry); err != nil {
			fmt.Fprintf(os.Stderr, "Error recovering %s: %v\n", localPath, err)
			os.Exit(1)
		}
		fmt.Printf("recovered %s\n", localPath)
		recovered++
	}

	// Restore directory metadata bottom-up
	for i := len(dirs) - 1; i >= 0; i-- {
		restoreMetadata(dirs[i].path, dirs[i].entry)
	}

	fmt.Printf("%d item(s) recovered\n", recovered)
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
