package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

func runLog(args []string) {
	for _, a := range args {
		if a == "--help" {
			fmt.Print(logHelp)
			return
		}
	}

	// No arguments: the store's own history — the journal. One line
	// per claim, byte-pure data on stdout.
	if len(args) < 1 {
		s, _ := store.OpenStore()
		if s == nil {
			fmt.Fprintf(os.Stderr, "Usage: c4 log [<file.c4m>...]\n")
			fmt.Fprintf(os.Stderr, "\nWith no arguments, lists the configured store's journal.\n")
			fmt.Fprintf(os.Stderr, "No store is configured (set C4_STORE or ~/.c4/config).\n")
			os.Exit(1)
		}
		r, ok := s.(interface{ Root() string })
		if !ok {
			fatalf("Error: the configured store has no local journal.")
		}
		claims, err := c4m.OpenJournal(r.Root()).Claims()
		if err != nil {
			fatalf("Error reading journal: %v", err)
		}
		// One line per section: the 1-based index (what c4 split takes),
		// a space, then the entry exactly as recorded.
		for i, c := range claims {
			fmt.Printf("%d %s\n", i+1, c.Line())
		}
		return
	}

	// Collect all sections across all files. A journal file (a copied
	// store's journal included) self-identifies by its @c4 journal
	// magic and lists exactly like the installed journal.
	var allSections []*c4m.PatchSection

	for _, path := range args {
		data, err := os.ReadFile(path)
		if err != nil {
			fatalf("Error reading %s: %v", path, err)
		}
		if c4m.IsJournal(data) {
			claims, err := c4m.DecodeJournal(data)
			if err != nil {
				fatalf("Error decoding %s: %v", path, err)
			}
			for i, c := range claims {
				fmt.Printf("%d %s\n", i+1, c.Line())
			}
			continue
		}
		sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
		if err != nil {
			fatalf("Error decoding %s: %v", path, err)
		}
		allSections = append(allSections, sections...)
	}

	if len(allSections) == 0 {
		return
	}

	// Display each section with summary stats. The chain resolves
	// incrementally — each section's state is the previous state plus
	// one patch — so a long history costs one apply per section, not a
	// from-scratch resolution per section.
	var prev *c4m.Manifest

	// A first-line bare ID names an external base: the block this chain
	// refines, outside these files. List the base as layer 0, resolved from
	// the store (transitively), and every section of the files as a patch
	// against it, so no block is presented as a base state. The indices of
	// the files' own sections are unchanged (c4 split takes them). A base
	// the store cannot supply is an error naming it, as in c4 patch.
	external := !allSections[0].BaseID.IsNil()
	var src store.Source
	if s, _ := store.OpenStore(); s != nil {
		src = s
	}
	if external {
		baseID := allSections[0].BaseID
		resolved, err := c4m.ResolveBaseID(baseID, src)
		if err != nil {
			fatalf("Error: %v\n"+
				"The first line names a base block this chain refines; it must be in the store.", err)
		}
		files, dirs := countEntriesBy(resolved.Entries)
		fmt.Printf("0  %s  (external base %s)  %d files, %d dirs\n", resolved.ComputeC4ID(), baseID, files, dirs)
		prev = resolved
	}

	for i, sec := range allSections {
		var current *c4m.Manifest
		if i == 0 && !external {
			current = c4m.ResolvePatchChain(allSections[:1], 1)
		} else {
			// The same fold c4 patch uses: a collapsed directory the
			// section deepens has its listing read from the store first.
			// The counts below compare against that deepened layer, so
			// children read from the store are not counted as added.
			deep, err := c4m.Deepen(prev, sec.Entries, src)
			if err != nil {
				fatalf("Error: layer %d: %v", i+1, err)
			}
			prev = deep
			current = c4m.ResolvePatchChain([]*c4m.PatchSection{{Entries: deep.Entries}, {Entries: sec.Entries}}, 0)
		}
		id := current.ComputeC4ID()

		if i == 0 && !external {
			// Base manifest.
			files, dirs := countEntriesBy(sec.Entries)
			fmt.Printf("%d  %s  (base)  %d files, %d dirs\n", i+1, id, files, dirs)
		} else {
			// Patch: show add/remove/modify counts.
			added, removed, modified := diffStats(prev, current)
			fmt.Printf("%d  %s  +%d -%d ~%d\n", i+1, id, added, removed, modified)
		}

		prev = current
	}
}

func countEntriesBy(entries []*c4m.Entry) (files, dirs int) {
	for _, e := range entries {
		if e.IsDir() {
			dirs++
		} else {
			files++
		}
	}
	return
}

func diffStats(old, new *c4m.Manifest) (added, removed, modified int) {
	oldMap := make(map[string]*c4m.Entry)
	buildEntryMap(old, oldMap)

	newMap := make(map[string]*c4m.Entry)
	buildEntryMap(new, newMap)

	for path, ne := range newMap {
		oe, exists := oldMap[path]
		if !exists {
			added++
		} else if ne.C4ID != oe.C4ID {
			modified++
		}
	}

	for path := range oldMap {
		if _, exists := newMap[path]; !exists {
			removed++
		}
	}

	return
}

func buildEntryMap(m *c4m.Manifest, out map[string]*c4m.Entry) {
	var dirStack []string
	for _, entry := range m.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
		path := ""
		if len(dirStack) > 0 && entry.Depth <= len(dirStack) {
			path = fmt.Sprintf("%s%s", joinStack(dirStack, entry.Depth), entry.Name)
		} else {
			path = entry.Name
		}
		out[path] = entry
	}
}

func joinStack(stack []string, depth int) string {
	if depth > len(stack) {
		depth = len(stack)
	}
	result := ""
	for i := 0; i < depth; i++ {
		result += stack[i]
	}
	return result
}
