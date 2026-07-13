package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/c4m"
)

// patch is text algebra: c4m chains in, c4m text out. It never touches
// directories — that is restore's job, behind restore's safety machine.
func runPatch(args []string) {
	fs := newFlags("patch")
	n := fs.intFlag("number", 'n', 0, "Resolve to section N (1-based; 0 = final state)")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Column-aligned output")
	fs.parse(args)

	if len(fs.args) == 0 {
		patchUsage()
		os.Exit(1)
	}

	for _, p := range fs.args {
		if isDirectory(p) {
			fatalf("patch composes descriptions; to change a directory: c4 restore <target> <dir>")
		}
	}

	runPatchChain(fs.args, *n, *ergonomic)
}

func patchUsage() {
	fmt.Fprintf(os.Stderr, `Usage: c4 patch [-n N] [-e] <chain.c4m>...

Resolve patch chains to c4m text on stdout (never touches directories).
Multiple files concatenate into one chain; -n N picks a section.

patch composes descriptions; to change a directory: c4 restore <target> <dir>
`)
}

// runPatchChain concatenates the arguments into one chain and resolves it.
func runPatchChain(paths []string, n int, ergonomic bool) {
	var allSections []*c4m.PatchSection

	for _, path := range paths {
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
		fatalf("Error: no content found")
	}

	manifest := c4m.ResolvePatchChain(allSections, n)
	outputManifest(manifest, ergonomic)
}

// resolveC4m loads a c4m file and resolves any patch chain to a final manifest.
func resolveC4m(path string) *c4m.Manifest {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("Error reading %s: %v", path, err)
	}

	sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
	if err != nil {
		// Not a patch chain -- try loading as plain manifest.
		m, err2 := loadManifest(path)
		if err2 != nil {
			fatalf("Error loading %s: %v", path, err2)
		}
		return m
	}

	if len(sections) == 0 {
		// No patch sections -- load as plain manifest.
		m, err := loadManifest(path)
		if err != nil {
			fatalf("Error loading %s: %v", path, err)
		}
		return m
	}

	return c4m.ResolvePatchChain(sections, 0)
}
