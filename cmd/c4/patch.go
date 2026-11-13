package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// patch is text algebra: c4m chains in, c4m text out. It never touches
// directories — that is restore's job, behind restore's safety machine.
func runPatch(args []string) {
	fs := newFlags("patch")
	fs.help(patchHelp)
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
	// interior[i]: section i's bare ID is a checkpoint inside its own file,
	// not the first line of a file (see verifyInteriorCheckpoints).
	var interior []bool

	for _, path := range paths {
		// "-" is stdin, as for every verb that takes a c4m, so a chain
		// composes: c4 id DIR | c4 patch - | c4
		data, err := readC4mArg(path)
		if err != nil {
			fatalf("Error reading %s: %v", path, err)
		}
		sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
		if err != nil {
			fatalf("Error decoding %s: %v", path, err)
		}
		for i := range sections {
			interior = append(interior, i > 0)
		}
		allSections = append(allSections, sections...)
	}

	if len(allSections) == 0 {
		fatalf("Error: no content found")
	}

	// A first-line bare ID names a block outside these files. Backfill it
	// from the store — transitively, so the newest block reconstructs every
	// prior — and fail loudly if it cannot be read. Prepending it as a
	// section keeps -n numbering relative to the file's own sections.
	if base := allSections[0].BaseID; !base.IsNil() {
		s, _ := store.OpenStore()
		resolved, err := c4m.ResolveBaseID(base, s)
		if err != nil {
			fatalf("Error: %v\n"+
				"The first line names a base block this chain refines; it must be in the store.", err)
		}
		allSections = append([]*c4m.PatchSection{{Entries: resolved.Entries}}, allSections...)
		interior = append([]bool{false}, interior...)
		if n > 0 {
			n++
		}
	}

	var src store.Source
	if s, _ := store.OpenStore(); s != nil {
		src = s
	}
	manifest, err := foldChain(allSections, interior, n, src)
	if err != nil {
		fatalf("Error: %v", err)
	}
	outputManifest(manifest, ergonomic)
}

// foldChain resolves sections up to section n (all when n is 0), one
// section at a time through c4m.FoldSection, the fold every chain resolver
// shares. Two things the plain ResolvePatchChain never did:
//
//   - A section that deepens a collapsed directory (a one-level base lists
//     a subdirectory as one line carrying its listing's ID) has that
//     listing fetched from src first. Before, the directory read as empty,
//     so a restated removal under it became an addition and patch printed a
//     description its own directory IDs contradict (c4 #140 fixed the
//     library fold; this CLI path still had it, 2026-11-13).
//   - Every bare ID inside a file is checked against the state the sections
//     before it resolve to (chain-vector/bad-checkpoint.c4m). The first line
//     of each file is not: in a concatenation it may be a state ID or the
//     byte ID of the block it refines, and which it must be is not settled.
func foldChain(sections []*c4m.PatchSection, interior []bool, n int, src store.Source) (*c4m.Manifest, error) {
	limit := len(sections)
	if n > 0 && n < limit {
		limit = n
	}
	if limit == 1 {
		return c4m.ResolvePatchChain(sections, 1), nil
	}
	state := &c4m.Manifest{Version: "1.0", Entries: sections[0].Entries}
	for i := 1; i < limit; i++ {
		sec := sections[i]
		if interior[i] && !sec.BaseID.IsNil() {
			if got := state.ComputeC4ID(); got != sec.BaseID {
				return nil, fmt.Errorf("%w: checkpoint before section %d names %s, the sections before it resolve to %s",
					c4m.ErrPatchIDMismatch, i+1, sec.BaseID, got)
			}
		}
		next, err := c4m.FoldSection(state, sec.Entries, src)
		if err != nil {
			return nil, err
		}
		state = next
	}
	return state, nil
}

// resolveC4m loads a c4m file through the one verifying loader:
// chains resolve to their final state and checkpoints are verified; a
// file ending with a bare ID is refused (format error, ruled 2026-11-08).
func resolveC4m(path string) *c4m.Manifest {
	if path == "-" {
		return manifestFromStdin()
	}
	m, err := loadManifest(path)
	if err != nil {
		fatalf("Error loading %s: %v", path, err)
	}
	return m
}

// readC4mArg reads a c4m argument's bytes: a path, or "-" for stdin.
func readC4mArg(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}
