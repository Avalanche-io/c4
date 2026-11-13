package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/Avalanche-io/c4"
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
	// checks[i] says how section i's bare ID is verified (see foldChain).
	var checks []idCheck
	var prevFile c4.ID // byte ID of the previous file's bytes

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
			switch {
			case i > 0:
				checks = append(checks, idCheck{kind: checkInterior})
			case !prevFile.IsNil():
				checks = append(checks, idCheck{kind: checkBoundary, prevFile: prevFile})
			default:
				checks = append(checks, idCheck{})
			}
		}
		allSections = append(allSections, sections...)
		prevFile = c4.Identify(bytes.NewReader(data))
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
		allSections = append([]*c4m.PatchSection{{Entries: resolved.Entries, RangeData: resolved.RangeData}}, allSections...)
		checks = append([]idCheck{{}}, checks...)
		if n > 0 {
			n++
		}
	}

	var src store.Source
	if s, _ := store.OpenStore(); s != nil {
		src = s
	}
	manifest, err := foldChain(allSections, checks, n, src)
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
//     before it resolve to (chain-vector/bad-checkpoint.c4m).
//   - A later file's first-line bare ID, the boundary between two files,
//     is accepted when it equals either the accumulated state's description
//     ID (the checkpoint form `c4 diff` writes) or the byte ID of the
//     preceding file (the block form of a shape-B chain), and refused
//     otherwise, naming both (ruled 2026-11-13). Both are what the spec's
//     bare ID means, "fetch this manifest by its C4 ID": a description ID
//     fetches the stored listing, a byte ID the block.
func foldChain(sections []*c4m.PatchSection, checks []idCheck, n int, src store.Source) (*c4m.Manifest, error) {
	limit := len(sections)
	if n > 0 && n < limit {
		limit = n
	}
	if limit == 1 {
		return c4m.ResolvePatchChain(sections, 1), nil
	}
	state := &c4m.Manifest{Version: "1.0", Entries: sections[0].Entries, RangeData: sections[0].RangeData}
	for i := 1; i < limit; i++ {
		sec := sections[i]
		if err := checks[i].verify(sec.BaseID, state, i+1); err != nil {
			return nil, err
		}
		// The section's ID lists join state's before the fold, which
		// keeps those of the ranges the result holds (lists are keyed by
		// their own C4 ID, so the union is safe). Through 2026-11-13
		// patch dropped every list.
		withLists := *state
		withLists.RangeData = unionRangeData(state.RangeData, sec.RangeData)
		next, err := c4m.FoldSection(&withLists, sec.Entries, src)
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

type checkKind int

const (
	checkNone     checkKind = iota // a file's first line naming an external base, or no ID
	checkInterior                  // a checkpoint inside a file
	checkBoundary                  // a later file's first line
)

type idCheck struct {
	kind     checkKind
	prevFile c4.ID
}

// verify checks a section's bare ID against the state the sections before
// it resolve to; section is its 1-based number in the chain.
func (c idCheck) verify(id c4.ID, state *c4m.Manifest, section int) error {
	if id.IsNil() || c.kind == checkNone {
		return nil
	}
	got := state.ComputeC4ID()
	switch c.kind {
	case checkInterior:
		if got != id {
			return fmt.Errorf("%w: checkpoint before section %d names %s, the sections before it resolve to %s",
				c4m.ErrPatchIDMismatch, section, id, got)
		}
	case checkBoundary:
		if got != id && c.prevFile != id {
			return fmt.Errorf("%w: the file starting at section %d names %s, which is neither the state the files before it resolve to (%s) nor the byte ID of the file before it (%s)",
				c4m.ErrPatchIDMismatch, section, id, got, c.prevFile)
		}
	}
	return nil
}

// unionRangeData is every ID list in a and b. Lists are keyed by their own
// C4 ID, so a key in both is the same list.
func unionRangeData(a, b map[c4.ID]string) map[c4.ID]string {
	if len(b) == 0 {
		return a
	}
	out := make(map[c4.ID]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
