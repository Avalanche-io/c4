package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/c4m"
)

func runLog(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 log <file.c4m>...\n")
		fmt.Fprintf(os.Stderr, "\nList patches in a c4m chain.\n")
		os.Exit(1)
	}

	// Collect all sections across all files.
	var allSections []*c4m.PatchSection

	for _, path := range args {
		data, err := os.ReadFile(path)
		if err != nil {
			fatalf("Error reading %s: %v", path, err)
		}
		sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
		if err != nil {
			fatalf("Error decoding %s: %v", path, err)
		}
		allSections = append(allSections, sections...)
	}

	if len(allSections) == 0 {
		fmt.Fprintf(os.Stderr, "No patches found.\n")
		return
	}

	// Display each section with summary stats.
	var prev *c4m.Manifest
	for i, sec := range allSections {
		current := c4m.ResolvePatchChain(allSections, i+1)
		id := current.ComputeC4ID()

		if i == 0 {
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
