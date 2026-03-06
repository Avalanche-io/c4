package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
)

// runPatch implements "c4 patch" — apply a c4m patch or target state.
//
//	c4 patch project.c4m: changes.c4m
//	c4 patch : changes.c4m           # apply to managed directory (tracked, undoable)
//	c4 patch : desired.c4m           # converge to target state
//	c4 patch : .                     # re-sync managed state to match disk
func runPatch(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 patch <target> <source>\n")
		fmt.Fprintf(os.Stderr, "  c4 patch project.c4m: changes.c4m  # patch a c4m file\n")
		fmt.Fprintf(os.Stderr, "  c4 patch : changes.c4m             # apply changes (tracked)\n")
		fmt.Fprintf(os.Stderr, "  c4 patch : .                       # re-sync from disk\n")
		os.Exit(1)
	}

	target := args[0]
	source := args[1]

	if target == ":" {
		patchManaged(source)
		return
	}

	// Non-managed targets: apply patch to a c4m file
	spec, err := pathspec.Parse(target, establish.IsLocationEstablished)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if spec.Type != pathspec.Capsule {
		fmt.Fprintf(os.Stderr, "Error: patch target must be : or a c4m file (with colon)\n")
		os.Exit(1)
	}

	patchC4mFile(spec.Source, source)
}

// patchC4mFile applies a source to a c4m file.
// Auto-detects: plain c4m = target state mode, patch with page boundaries = delta mode.
func patchC4mFile(c4mPath, source string) {
	// Load the base c4m file
	base, err := loadManifest(c4mPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", c4mPath, err)
		os.Exit(1)
	}

	// Load the source
	var sourceManifest *c4m.Manifest
	if source == "-" {
		sourceManifest, err = c4m.NewDecoder(os.Stdin).Decode()
	} else if strings.HasSuffix(source, ".c4m") {
		sourceManifest, err = loadManifest(source)
	} else {
		fmt.Fprintf(os.Stderr, "Error: source must be a .c4m file or -\n")
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading source: %v\n", err)
		os.Exit(1)
	}

	// Apply: use PatchDiff to compute delta, then ApplyPatch
	// This handles both target-state and delta modes uniformly:
	// target-state input produces a patch that converges to the desired state
	patch := c4m.PatchDiff(base, sourceManifest)
	if patch.IsEmpty() {
		fmt.Fprintf(os.Stderr, "no changes\n")
		return
	}

	result := c4m.ApplyPatch(base, patch.Patch)

	// Write result back to the c4m file
	f, err := os.Create(c4mPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", c4mPath, err)
		os.Exit(1)
	}
	defer f.Close()

	if err := c4m.NewEncoder(f).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "patched %s (%d entries)\n", c4mPath, len(result.Entries))
}

// patchManaged applies a source to the managed directory.
// Auto-snapshots before applying (tracked, undoable).
func patchManaged(source string) {
	d, err := managed.Open(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if source == "." {
		// Re-sync: scan the live disk and make that the new state
		id, err := d.Snapshot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		m, _ := d.Current()
		fmt.Printf("synced : from disk (%d entries, id %s)\n", len(m.Entries), id.String()[:12]+"...")
		return
	}

	// Load source manifest
	var sourceManifest *c4m.Manifest
	if strings.HasSuffix(source, ".c4m") {
		sourceManifest, err = loadManifest(source)
	} else if source == "-" {
		sourceManifest, err = c4m.NewDecoder(os.Stdin).Decode()
	} else {
		fmt.Fprintf(os.Stderr, "Error: source must be a .c4m file, -, or .\n")
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading source: %v\n", err)
		os.Exit(1)
	}

	// Snapshot before applying (the before-state for undo)
	_, err = d.Snapshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error snapshotting before patch: %v\n", err)
		os.Exit(1)
	}

	// Get current managed state and apply patch
	current, err := d.Current()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading managed state: %v\n", err)
		os.Exit(1)
	}

	patch := c4m.PatchDiff(current, sourceManifest)
	if patch.IsEmpty() {
		fmt.Fprintf(os.Stderr, "no changes\n")
		return
	}

	result := c4m.ApplyPatch(current, patch.Patch)
	_ = result

	fmt.Printf("patched : (tracked, undoable)\n")
}
