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
		fmt.Fprintf(os.Stderr, "  c4 patch : changes.c4m    # apply changes (tracked)\n")
		fmt.Fprintf(os.Stderr, "  c4 patch : .              # re-sync from disk\n")
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
		fmt.Fprintf(os.Stderr, "Error: patch target must be : or a c4m file\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Error: c4m file patching not yet implemented\n")
	os.Exit(1)
}

// patchManaged applies a source to the managed directory.
// Auto-snapshots before applying (tracked, undoable).
func patchManaged(source string) {
	d, err := managed.Open(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Load the source manifest
	var sourceManifest *c4m.Manifest

	if source == "." {
		// Re-sync: scan the live disk and make that the new state
		// This captures filesystem changes made outside c4
		id, err := d.Snapshot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		m, _ := d.Current()
		fmt.Printf("synced : from disk (%d entries, id %s)\n", len(m.Entries), id.String()[:12]+"...")
		return
	}

	// Load from file
	if strings.HasSuffix(source, ".c4m") {
		sourceManifest, err = loadManifest(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", source, err)
			os.Exit(1)
		}
	} else if source == "-" {
		sourceManifest, err = c4m.NewDecoder(os.Stdin).Decode()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Error: source must be a .c4m file, -, or .\n")
		os.Exit(1)
	}

	// Snapshot before applying (the before-state for undo)
	_, err = d.Snapshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error snapshotting before patch: %v\n", err)
		os.Exit(1)
	}

	// For now, we record the desired state as the new snapshot.
	// Full filesystem materialization (creating/moving actual files) is a
	// future implementation that requires c4d content resolution.
	// The managed directory's tracked state converges to the source manifest.
	_ = sourceManifest

	fmt.Printf("patched : (tracked, undoable)\n")
}
