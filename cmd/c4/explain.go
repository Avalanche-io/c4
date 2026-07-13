package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

func runExplain(args []string) {
	if len(args) == 0 {
		explainUsage()
		return
	}

	switch args[0] {
	case "id":
		runExplainID(args[1:])
	case "diff":
		runExplainDiff(args[1:])
	case "patch":
		runExplainPatch(args[1:])
	case "restore":
		runExplainRestore(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "c4 explain: unknown command %q\n", args[0])
		explainUsage()
		os.Exit(1)
	}
}

func explainUsage() {
	fmt.Print(`c4 explain — see what a command would do, in plain language

Usage:
  c4 explain id <path>                  What does this directory or c4m file contain?
  c4 explain diff <old> <new>           What changed between two states?
  c4 explain patch <chain.c4m>...       What state does this chain resolve to?
  c4 explain restore <target> [<dest>]  What would a restore change?

The explain command never modifies any files. It is always safe to run.
`)
}

// runExplainID describes a directory or c4m file in human terms.
func runExplainID(args []string) {
	fs := newFlags("explain id")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode: s/1=structure, m/2=metadata, f/3=full")
	fs.parse(args)

	if len(fs.args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: c4 explain id <path>\n")
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	p := fs.args[0]
	info, err := os.Lstat(p)
	if err != nil {
		fatalf("Error: %v", err)
	}

	var m *c4m.Manifest
	var label string

	if info.IsDir() {
		label = p
		gen := scan.NewGeneratorWithOptions(scan.WithMode(mode))
		m, err = gen.GenerateFromPath(p)
		if err != nil {
			fatalf("Error scanning %s: %v", p, err)
		}
	} else {
		label = filepath.Base(p)
		m, err = loadManifest(p)
		if err != nil {
			fatalf("Error loading %s: %v", p, err)
		}
	}

	files, dirs, totalSize := manifestStats(m)
	fmt.Printf("Scanning %s: %s in %s (%s)\n", label,
		pluralize(files, "file"), pluralize(dirs, "directory", "directories"),
		formatBytes(totalSize))
	fmt.Println()
	fmt.Println("This produces a c4m file — a text description of every file:")
	fmt.Println("  permissions  timestamp  size  name  identity")
	fmt.Println()
	fmt.Printf("Save it:  c4 id %s > %s\n", p, suggestC4mName(p))
	fmt.Printf("Diff it:  c4 diff %s %s\n", suggestC4mName(p), p)
}

// runExplainDiff shows a human-readable summary of changes between two states.
func runExplainDiff(args []string) {
	fs := newFlags("explain diff")
	fs.parse(args)

	if len(fs.args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 explain diff <old> <new>\n")
		os.Exit(1)
	}

	oldArg, newArg := fs.args[0], fs.args[1]
	oldManifest, newManifest := smartResolve(oldArg, newArg)

	oldFiles, _, _ := manifestStats(oldManifest)
	newFiles, _, _ := manifestStats(newManifest)

	fmt.Printf("Comparing %s (%s) against %s (%s):\n",
		oldArg, pluralize(oldFiles, "file"),
		newArg, pluralize(newFiles, "file"))

	// Build path-keyed maps for detailed comparison.
	oldMap := make(map[string]*c4m.Entry)
	buildEntryMap(oldManifest, oldMap)
	newMap := make(map[string]*c4m.Entry)
	buildEntryMap(newManifest, newMap)

	type modifiedEntry struct {
		path    string
		oldSize int64
		newSize int64
	}
	type simpleEntry struct {
		path string
		size int64
	}

	var modified []modifiedEntry
	var added []simpleEntry
	var removed []simpleEntry
	unchanged := 0

	for path, ne := range newMap {
		if ne.IsDir() {
			continue
		}
		oe, exists := oldMap[path]
		if !exists {
			added = append(added, simpleEntry{path, ne.Size})
			continue
		}
		if ne.C4ID != oe.C4ID {
			modified = append(modified, modifiedEntry{path, oe.Size, ne.Size})
		} else {
			unchanged++
		}
	}

	for path, oe := range oldMap {
		if oe.IsDir() {
			continue
		}
		if _, exists := newMap[path]; !exists {
			removed = append(removed, simpleEntry{path, oe.Size})
		}
	}

	fmt.Println()

	if len(modified) > 0 {
		fmt.Printf("  %s modified\n", pluralize(len(modified), "file"))
		for _, e := range modified {
			fmt.Printf("    %-30s %s -> %s\n", e.path, formatBytes(e.oldSize), formatBytes(e.newSize))
		}
	}
	if len(added) > 0 {
		fmt.Printf("  %s added\n", pluralize(len(added), "file"))
		for _, e := range added {
			fmt.Printf("    %-30s %s\n", e.path, formatBytes(e.size))
		}
	}
	if len(removed) > 0 {
		fmt.Printf("  %s removed\n", pluralize(len(removed), "file"))
		for _, e := range removed {
			fmt.Printf("    %-30s %s\n", e.path, formatBytes(e.size))
		}
	}

	if len(modified) == 0 && len(added) == 0 && len(removed) == 0 {
		fmt.Println("  No differences.")
	}

	if unchanged > 0 {
		fmt.Printf("\n%s unchanged.\n", pluralize(unchanged, "file"))
	}
}

// runExplainPatch narrates what patch is: text algebra over chains.
func runExplainPatch(args []string) {
	fs := newFlags("explain patch")
	fs.parse(args)

	if len(fs.args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: c4 explain patch <chain.c4m>...\n")
		os.Exit(1)
	}

	total := 0
	for _, p := range fs.args {
		if isDirectory(p) {
			fmt.Printf("%s is a directory. patch composes descriptions and never touches\n", p)
			fmt.Println("directories; to change a directory, use restore:")
			fmt.Printf("  c4 explain restore <target> %s\n", p)
			return
		}
		data, err := os.ReadFile(p)
		if err != nil {
			fatalf("Error reading %s: %v", p, err)
		}
		sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
		if err != nil {
			fatalf("Error decoding %s: %v", p, err)
		}
		fmt.Printf("%s: %s\n", p, pluralize(len(sections), "section"))
		total += len(sections)
	}

	final := resolveC4m(fs.args[0])
	if len(fs.args) > 1 {
		fmt.Printf("\nThe %d files concatenate into one chain of %s.\n",
			len(fs.args), pluralize(total, "section"))
	}
	files, _, size := manifestStats(final)
	fmt.Printf("\nResolved to the final state, the chain describes %s (%s).\n",
		pluralize(files, "file"), formatBytes(size))
	fmt.Println()
	fmt.Println("c4 patch writes that state as c4m text to stdout — it never touches")
	fmt.Println("directories (-n N picks an earlier section). To make a directory")
	fmt.Println("match it: c4 restore <target> <dir>")
}

// runExplainRestore shows a human-readable restore plan.
func runExplainRestore(args []string) {
	fs := newFlags("explain restore")
	fs.parse(args)

	if len(fs.args) == 0 || len(fs.args) > 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 explain restore <target> [<dest>]\n")
		os.Exit(1)
	}

	targetArg := fs.args[0]
	targetManifest := resolveManifestOrDir(targetArg, scan.ModeFull)
	targetFiles, _, targetSize := manifestStats(targetManifest)

	// Single arg: describe the target without a plan.
	if len(fs.args) == 1 {
		fmt.Printf("Target: %s (%s, %s)\n", targetArg,
			pluralize(targetFiles, "file"), formatBytes(targetSize))
		fmt.Println()
		fmt.Println("Provide a destination directory to see the restore plan:")
		fmt.Printf("  c4 explain restore %s ./dest/\n", targetArg)
		return
	}

	destArg := fs.args[1]

	// Load or scan the destination.
	var destManifest *c4m.Manifest
	if info, serr := os.Stat(destArg); serr == nil && info.IsDir() {
		destManifest = guidedScan(destArg, targetManifest, scan.ModeFull)
	} else {
		destManifest = c4m.NewManifest()
	}

	fmt.Printf("Restoring %s to match %s:\n", destArg, targetArg)

	// Compare using entry maps — this works regardless of store availability.
	targetMap := make(map[string]*c4m.Entry)
	buildEntryMap(targetManifest, targetMap)
	destMap := make(map[string]*c4m.Entry)
	buildEntryMap(destManifest, destMap)

	var createCount, updateCount, removeCount, skipCount int
	var createSize, removeSize int64

	for path, te := range targetMap {
		if te.IsDir() {
			continue
		}
		de, exists := destMap[path]
		if !exists {
			createCount++
			if te.Size > 0 {
				createSize += te.Size
			}
			continue
		}
		if !te.C4ID.IsNil() && te.C4ID != de.C4ID {
			updateCount++
		} else {
			skipCount++
		}
	}

	for path, de := range destMap {
		if de.IsDir() {
			continue
		}
		if _, exists := targetMap[path]; !exists {
			removeCount++
			if de.Size > 0 {
				removeSize += de.Size
			}
		}
	}

	fmt.Println()
	if createCount > 0 {
		fmt.Printf("  %s to create (%s)\n",
			pluralize(createCount, "file"), formatBytes(createSize))
	}
	if updateCount > 0 {
		fmt.Printf("  %s to update\n", pluralize(updateCount, "file"))
	}
	if removeCount > 0 {
		fmt.Printf("  %s to remove (%s)\n",
			pluralize(removeCount, "file"), formatBytes(removeSize))
	}
	if skipCount > 0 {
		fmt.Printf("  %s already correct — skipping\n", pluralize(skipCount, "file"))
	}

	if createCount == 0 && updateCount == 0 && removeCount == 0 {
		fmt.Println("  Already up to date — nothing to do.")
	}

	// Store availability check.
	s, _ := store.OpenStore()
	fmt.Println()
	if s != nil {
		storePath := store.DefaultStorePath()
		// Check which target IDs need content (creates + updates).
		var missingCount int
		var missingSize int64
		for path, te := range targetMap {
			if te.IsDir() || te.C4ID.IsNil() {
				continue
			}
			de, exists := destMap[path]
			if exists && te.C4ID == de.C4ID {
				continue // already correct
			}
			if !s.Has(te.C4ID) {
				missingCount++
				if te.Size > 0 {
					missingSize += te.Size
				}
			}
		}

		if missingCount == 0 {
			fmt.Printf("Store: %s (all content available)\n", storePath)
		} else {
			fmt.Printf("Store: %s\n", storePath)
			fmt.Printf("Missing: %s (%s) — not in store\n",
				pluralize(missingCount, "file"), formatBytes(missingSize))
		}
	} else {
		fmt.Println("Store: not configured")
	}

	if createCount > 0 || updateCount > 0 || removeCount > 0 {
		fmt.Println()
		fmt.Printf("Dry run (prints the plan):  c4 restore %s %s\n", targetArg, destArg)
		fmt.Printf("Apply (undo-safely):        c4 restore --force %s %s\n", targetArg, destArg)
		fmt.Println("--force snapshots the destination first; stdout line 1 is the undo handle.")
	}
}

// manifestStats returns file count, directory count, and total file size.
func manifestStats(m *c4m.Manifest) (files, dirs int, totalSize int64) {
	for _, e := range m.Entries {
		if e.IsDir() {
			dirs++
		} else {
			files++
			if e.Size > 0 {
				totalSize += e.Size
			}
		}
	}
	return
}

// formatBytes formats a byte count with comma separators and a "bytes" suffix.
func formatBytes(n int64) string {
	if n < 0 {
		return "unknown size"
	}
	return commaFormat(n) + " bytes"
}

// commaFormat inserts commas into an integer for readability.
func commaFormat(n int64) string {
	if n < 0 {
		return "-" + commaFormat(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	// Insert commas from right to left.
	var result []byte
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(ch))
	}
	return string(result)
}

// pluralize returns "N item" or "N items" as appropriate.
// Optional second argument provides custom plural form.
func pluralize(count int, singular string, plural ...string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	p := singular + "s"
	if len(plural) > 0 {
		p = plural[0]
	}
	return fmt.Sprintf("%s %s", commaFormat(int64(count)), p)
}

// suggestC4mName suggests a c4m filename for a given path.
func suggestC4mName(p string) string {
	base := filepath.Base(filepath.Clean(p))
	if base == "." || base == "/" {
		return "output.c4m"
	}
	return base + ".c4m"
}
