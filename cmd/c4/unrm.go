package main

import (
	"fmt"
	"io"
	"net/http"
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

	recovered := 0
	for _, m := range matched {
		localPath := m.fullPath

		if m.entry.IsDir() {
			if err := os.MkdirAll(localPath, m.entry.Mode.Perm()|0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", localPath, err)
				os.Exit(1)
			}
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

		// Regular file — fetch content
		dir := filepath.Dir(localPath)
		if dir != "." {
			os.MkdirAll(dir, 0755)
		}

		if m.entry.C4ID.IsNil() {
			// No content — create empty file
			f, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, m.entry.Mode.Perm()|0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", localPath, err)
				os.Exit(1)
			}
			f.Close()
			fmt.Printf("recovered %s (empty)\n", localPath)
			recovered++
			continue
		}

		// Fetch from c4d
		resp, err := c4dClient.Get(c4dAddr() + "/" + m.entry.C4ID.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching %s: %v\n", localPath, err)
			os.Exit(1)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			fmt.Fprintf(os.Stderr, "Error: c4d returned %s for %s (C4 ID %s)\n", resp.Status, localPath, m.entry.C4ID)
			os.Exit(1)
		}

		f, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, m.entry.Mode.Perm()|0644)
		if err != nil {
			resp.Body.Close()
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", localPath, err)
			os.Exit(1)
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			f.Close()
			resp.Body.Close()
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", localPath, err)
			os.Exit(1)
		}
		f.Close()
		resp.Body.Close()
		fmt.Printf("recovered %s\n", localPath)
		recovered++
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
