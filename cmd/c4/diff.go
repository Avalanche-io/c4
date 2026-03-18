package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
	"github.com/Avalanche-io/c4/store"
)

func runDiff(args []string) {
	fs := newFlags("diff")
	storeFlag := fs.boolFlag("store", 's', false, "Store content in the configured store")
	reverseFlag := fs.boolFlag("reverse", 'r', false, "Reverse: diff against pre-patch state from a changeset")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Output ergonomic form")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode for directories: s/m/f")
	fs.parse(args)

	if len(fs.args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 diff [-r] [-s] [-e] [-m mode] <old> <new>\n")
		fmt.Fprintf(os.Stderr, "\nProduce a c4m diff (patch). Each argument can be a c4m file or directory.\n")
		fmt.Fprintf(os.Stderr, "  -r  With a changeset as first arg: diff against the pre-patch state\n")
		fmt.Fprintf(os.Stderr, "      With two manifests/dirs: swap old and new\n")
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	// Reverse mode with a changeset: extract OldID, load pre-patch manifest from store.
	if *reverseFlag && !isDirectory(fs.args[0]) && isChangesetFile(fs.args[0]) {
		runDiffReverse(fs.args[0], fs.args[1], mode, *ergonomic)
		return
	}

	oldArg, newArg := fs.args[0], fs.args[1]
	if *reverseFlag {
		oldArg, newArg = newArg, oldArg
	}

	oldManifest := resolveManifestOrDir(oldArg, mode)
	newManifest := resolveManifestOrDir(newArg, mode)

	// Store content from directory arguments if requested.
	if *storeFlag && mode == scan.ModeFull {
		for _, p := range fs.args {
			if !isDirectory(p) {
				continue
			}
			storeManifestContent(resolveManifestOrDir(p, mode), p)
		}
	}

	outputDiff(oldManifest, newManifest, *ergonomic)
}

// runDiffReverse handles `c4 diff -r changeset.c4m dir/`.
// Loads the pre-patch manifest from the store and diffs the directory against it.
func runDiffReverse(changesetPath, dirPath string, mode scan.ScanMode, ergonomic bool) {
	// Read the changeset to extract OldID.
	data, err := os.ReadFile(changesetPath)
	if err != nil {
		fatalf("Error reading %s: %v", changesetPath, err)
	}
	sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
	if err != nil {
		fatalf("Error decoding %s: %v", changesetPath, err)
	}
	if len(sections) == 0 {
		fatalf("Error: changeset is empty")
	}

	oldID := sections[0].BaseID
	if oldID.IsNil() {
		base := &c4m.Manifest{Version: "1.0", Entries: sections[0].Entries}
		oldID = base.ComputeC4ID()
	}

	// Load pre-patch manifest from store.
	s, _ := store.OpenConfigured()
	if s == nil || !s.Has(oldID) {
		fatalf("Error: pre-patch manifest %s not found in store\n"+
			"Was the original patch run with -s?", oldID)
	}

	rc, err := s.Open(oldID)
	if err != nil {
		fatalf("Error loading pre-patch manifest: %v", err)
	}
	prePatchManifest, err := c4m.NewDecoder(rc).Decode()
	rc.Close()
	if err != nil {
		fatalf("Error decoding pre-patch manifest: %v", err)
	}

	// Diff current state against pre-patch state.
	currentManifest := resolveManifestOrDir(dirPath, mode)
	outputDiff(currentManifest, prePatchManifest, ergonomic)
}

// isChangesetFile returns true if the file starts with a bare C4 ID line
// (indicating it's a changeset/patch file, not a plain manifest).
func isChangesetFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Check if the first non-blank line is a bare C4 ID.
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		return len(trimmed) == 90 && trimmed[0] == 'c' && trimmed[1] == '4'
	}
	return false
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
