package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

// runDiff compares two states — directories, c4m files, or store IDs —
// and emits a c4m patch on stdout. Directories scan at content level,
// or at the other side's level when that side is a description,
// folding exactly what it folds.
func runDiff(args []string) {
	fs := newFlags("diff")
	fs.help(diffHelp)
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Column-aligned output")
	fs.parse(args)

	if len(fs.args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 diff [-e] <old> <new>\n")
		fmt.Fprintf(os.Stderr, "\nProduce a c4m diff (patch). Sides: directories, c4m files, or store IDs.\n")
		os.Exit(1)
	}

	oldArg, newArg := fs.args[0], fs.args[1]
	oldManifest, newManifest := smartResolve(oldArg, newArg)
	outputDiff(oldManifest, newManifest, *ergonomic)
}

// resolveDiffSide loads a non-directory diff side: a store address
// (ID/path allowed; the resolved object must be a listing) or a c4m
// file. Chains resolve to their final state.
func resolveDiffSide(arg string) *c4m.Manifest {
	if first := strings.SplitN(arg, "/", 2)[0]; looksLikeC4ID(first) {
		id, err := c4.Parse(first)
		if err != nil {
			fatalf("Error: invalid C4 ID: %v", err)
		}
		s, _ := store.OpenStore()
		if s == nil {
			fatalf("Error: %s is a store address and no store is configured", arg)
		}
		id, _ = storeDescend(s, id, arg[len(first):])
		m := verifiedManifestFromStore(s, id)
		if m == nil {
			fatalf("Error: %s must resolve to a listing", arg)
		}
		return expandIfRecord(m, s)
	}
	return resolveC4m(arg)
}

// descLevel is the scan level a description implies for the directory
// on the other side: content-form descriptions compare at content
// level; anything else compares at full fidelity.
func descLevel(m *c4m.Manifest) scan.ScanMode {
	if isContentLevel(m) {
		return scan.ModeContent
	}
	return scan.ModeFull
}

// smartResolve loads both sides. A directory scans at the other side's
// level when that side is a description (using it as a guide so only
// changed files rehash), at content level otherwise.
func smartResolve(oldArg, newArg string) (*c4m.Manifest, *c4m.Manifest) {
	oldIsDir := isDirectory(oldArg)
	newIsDir := isDirectory(newArg)

	// Both directories: compare at content level.
	if oldIsDir && newIsDir {
		return resolveManifestOrDir(oldArg, scan.ModeContent),
			resolveManifestOrDir(newArg, scan.ModeContent)
	}

	// Both descriptions: no scanning at all.
	if !oldIsDir && !newIsDir {
		return resolveDiffSide(oldArg), resolveDiffSide(newArg)
	}

	// One description, one directory: the description sets the level.
	var ref *c4m.Manifest
	var dirPath string
	if oldIsDir {
		ref = resolveDiffSide(newArg)
		dirPath = oldArg
	} else {
		ref = resolveDiffSide(oldArg)
		dirPath = newArg
	}
	// A full-form description guides the scan (unchanged size+mtime
	// reuse its IDs); a content-form one records no timestamps to
	// trust, so the directory hashes in full at content level.
	var dirManifest *c4m.Manifest
	if level := descLevel(ref); level == scan.ModeContent {
		dirManifest = resolveManifestOrDir(dirPath, scan.ModeContent)
	} else {
		dirManifest = guidedScan(dirPath, ref, level)
	}

	if oldIsDir {
		return dirManifest, ref
	}
	return ref, dirManifest
}

// outputDiff computes and prints a diff between two manifests.
func outputDiff(oldManifest, newManifest *c4m.Manifest, ergonomic bool) {
	result := c4m.PatchDiff(oldManifest, newManifest)
	if result.IsEmpty() {
		return
	}

	fmt.Println(result.OldID)

	enc := c4m.NewEncoder(os.Stdout)
	if ergonomic {
		enc.SetPretty(true)
	}
	enc.Encode(result.Patch)

	fmt.Println(result.NewID)
}
