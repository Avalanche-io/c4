package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/reconcile"
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

// runRestore implements the sole tree-writing verb of the snapshot
// loop (design/snapshot-loop): dry-run by default; `--force` applies.
// Stdout is exactly two lines under --force — line 1 the pre-image ID
// (the undo handle, durable and journaled before the first destructive
// operation), line 2 the as-built ID after verification — and nothing
// under dry run. Exit: 0 verified; 1 pre-flight refusal (nothing
// touched); 2 declared omissions (incomplete target entries kept as
// found); 3 curable failure (cure and re-run).
func runRestore(args []string) {
	fs := newFlags("restore")
	force := fs.boolFlag("force", 0, false, "Apply the restore (default is a dry run)")
	fs.parse(args)

	if len(fs.args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 restore [--force] <snapshot-id|file.c4m> <dir>\n")
		fmt.Fprintf(os.Stderr, "\nReconcile <dir> to a recorded state. Dry run unless --force.\n")
		fmt.Fprintf(os.Stderr, "With --force, line 1 is the pre-image ID (the undo handle),\n")
		fmt.Fprintf(os.Stderr, "line 2 the as-built ID after verification.\n")
		fmt.Fprintf(os.Stderr, "\nSCRIPTING\n")
		fmt.Fprintf(os.Stderr, "  IDS=$(c4 restore --force \"$SNAP\" dir/)   # two lines, each a bare ID\n")
		fmt.Fprintf(os.Stderr, "  UNDO=$(printf '%%s' \"$IDS\" | head -1)     # line 1 feeds straight back:\n")
		fmt.Fprintf(os.Stderr, "  c4 restore --force \"$UNDO\" dir/          # ...as the next restore target\n")
		os.Exit(1)
	}
	targetArg, destArg := fs.args[0], fs.args[1]

	s := getOrSetupStore()
	if s == nil {
		fatalf("Error: restore requires a configured store (set C4_STORE or ~/.c4/config)")
	}

	target := resolveRestoreTarget(s, targetArg)

	// Destination per OS path resolution: an existing non-directory
	// refuses; an absent path is created (with parents) on --force.
	destExists := false
	if info, err := os.Stat(destArg); err == nil {
		if !info.IsDir() {
			fatalf("Error: %s exists and is not a directory", destArg)
		}
		destExists = true
	}

	// Pre-flight closure check: every content-bearing target entry must
	// be servable from the store. Missing content lists and refuses —
	// nothing is touched. Incomplete entries (null IDs) are declared;
	// they will be kept as found and own exit 2.
	var missing []c4.ID
	var incomplete []string
	seen := make(map[c4.ID]bool)
	for _, e := range target.Entries {
		if e.IsDir() {
			continue
		}
		if e.Target != "" { // symlink: no store object needed
			continue
		}
		if e.C4ID.IsNil() {
			incomplete = append(incomplete, e.Name)
			continue
		}
		if seen[e.C4ID] {
			continue
		}
		seen[e.C4ID] = true
		if !s.Has(e.C4ID) {
			missing = append(missing, e.C4ID)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Missing content for %d entries; nothing restored:\n", len(missing))
		for _, id := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", id)
		}
		os.Exit(1)
	}
	for _, name := range incomplete {
		fmt.Fprintf(os.Stderr, "incomplete entry kept as found: %s\n", name)
	}

	// The verification level is the target's own knowledge level: a
	// content-projection target verifies content identity; a
	// full-fidelity target verifies recorded metadata too.
	level := scan.ModeFull
	if isContentLevel(target) {
		level = scan.ModeContent
	}

	if !*force {
		plan := planRestore(s, target, destArg, destExists)
		fmt.Fprintf(os.Stderr, "dry run: %d operations planned for %s (use --force to apply)\n",
			len(plan.Operations), destArg)
		for _, op := range plan.Operations {
			fmt.Fprintf(os.Stderr, "  %s %s\n", opName(op.Type), op.Path)
		}
		return
	}

	// Pre-image: the destination's full-fidelity prior state, durable
	// and journaled BEFORE the first destructive operation. Its ID is
	// line 1 — the undo handle. An absent or empty destination's
	// pre-image is the empty description constant.
	preStart := time.Now().UTC()
	var preManifest *c4m.Manifest
	if destExists {
		preManifest = scanDirectory(destArg, scan.ModeFull, false, false, nil, "", nil, time.Time{}, false)
	} else {
		preManifest = c4m.NewManifest()
		if err := os.MkdirAll(destArg, 0755); err != nil {
			fatalf("Error creating %s: %v", destArg, err)
		}
	}
	storeManifestContent(preManifest, destArg, preStart)
	preID := preManifest.ComputeC4ID()
	fmt.Println(preID) // line 1: printed only after the journaled barrier above

	// Reconcile to the target, pulling content from the store.
	plan := planRestore(s, target, destArg, true)
	r := restoreReconciler(s, preManifest, destArg)
	result, err := r.Apply(plan, destArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error applying restore: %v\n", err)
		os.Exit(3)
	}
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  error: %v\n", e)
		}
		fmt.Fprintf(os.Stderr, "restore incomplete: cure the failures and re-run\n")
		os.Exit(3)
	}

	// Verify at the target's knowledge level: the as-built tree must
	// recompute to the target's own root ID. Line 2 prints only when
	// recomputation reproduces it.
	built := scanDirectory(destArg, level, false, false, nil, "", nil, time.Time{}, false)
	builtID := built.ComputeC4ID()
	targetID := target.ComputeC4ID()
	if builtID != targetID {
		fmt.Fprintf(os.Stderr, "verification failed: as-built %s != target %s\nre-run the restore; the pre-image %s is durable\n",
			builtID, targetID, preID)
		os.Exit(3)
	}
	fmt.Println(builtID) // line 2: the as-built ID, verified

	if len(incomplete) > 0 {
		os.Exit(2)
	}
}

// resolveRestoreTarget loads a restore target: a store address
// (expanded through stored directory records) or a .c4m description
// file. Restore targets are descriptions only.
func resolveRestoreTarget(s store.Store, arg string) *c4m.Manifest {
	if looksLikeC4ID(arg) {
		id, err := c4.Parse(arg)
		if err != nil {
			fatalf("Error: invalid C4 ID: %v", err)
		}
		if !s.Has(id) {
			fatalf("Error: %s is not in the store", id)
		}
		m := manifestFromStore(s, id)
		m = expandIfRecord(m, s)
		validateRevertTarget(m)
		return m
	}
	if strings.HasSuffix(arg, ".c4m") {
		m, err := loadManifest(arg)
		if err != nil {
			fatalf("Error loading %s: %v", arg, err)
		}
		m = expandIfRecord(m, s)
		return m
	}
	fatalf("Error: restore targets are descriptions only — a snapshot ID or a .c4m file, not %s", arg)
	return nil
}

// isContentLevel reports whether every entry carries null mode and
// null timestamp — the content projection's signature.
func isContentLevel(m *c4m.Manifest) bool {
	if len(m.Entries) == 0 {
		return false
	}
	for _, e := range m.Entries {
		if e.Mode != 0 || !e.Timestamp.Equal(c4m.NullTimestamp()) {
			return false
		}
	}
	return true
}

// planRestore plans the reconcile of dest toward target, with the
// store as the content source.
func planRestore(s store.Store, target *c4m.Manifest, dest string, destExists bool) *reconcile.Plan {
	var current *c4m.Manifest
	if destExists {
		current = guidedScan(dest, target, scan.ModeFull)
	} else {
		current = c4m.NewManifest()
	}
	r := restoreReconciler(s, current, dest)
	plan, err := r.Plan(target, dest)
	if err != nil {
		fatalf("Error planning restore: %v", err)
	}
	return plan
}

func restoreReconciler(s store.Store, current *c4m.Manifest, dest string) *reconcile.Reconciler {
	return reconcile.New(
		reconcile.WithTrustedMetadata(true),
		reconcile.WithSyncMode(store.SyncBatch),
		reconcile.WithSource(reconcile.NewDirSource(current, dest)),
		reconcile.WithSource(s),
	)
}

// validateRevertTarget refuses a target whose description is
// incomplete: a directory entry with content (a non-empty ID) but no
// children after expansion means its record is missing from the store —
// reconciling toward it would silently delete that directory's contents.
func validateRevertTarget(m *c4m.Manifest) {
	empty := c4.Identify(bytes.NewReader(nil))
	for _, e := range m.Entries {
		if !e.IsDir() || e.C4ID.IsNil() || e.C4ID == empty {
			continue
		}
		if len(m.Children(e)) == 0 {
			fatalf("Error: target incomplete: no stored record for directory %s (%s)", e.Name, e.C4ID)
		}
	}
}

// manifestFromStore loads and decodes a description stored by a
// snapshot (c4 id -s) or a restore pre-image.
func manifestFromStore(s store.Store, id c4.ID) *c4m.Manifest {
	if !s.Has(id) {
		fatalf("Error: description %s not found in store", id)
	}
	rc, err := s.Open(id)
	if err != nil {
		fatalf("Error loading description: %v", err)
	}
	defer rc.Close()
	m, err := c4m.NewDecoder(rc).Decode()
	if err != nil {
		fatalf("Error decoding description: %v", err)
	}
	return m
}

// opName returns a human-readable name for a reconcile operation type.
func opName(op reconcile.Op) string {
	switch op {
	case reconcile.OpMkdir:
		return "mkdir"
	case reconcile.OpCreate:
		return "create"
	case reconcile.OpMove:
		return "move"
	case reconcile.OpSymlink:
		return "symlink"
	case reconcile.OpChmod:
		return "chmod"
	case reconcile.OpChtimes:
		return "chtimes"
	case reconcile.OpRemove:
		return "remove"
	case reconcile.OpRmdir:
		return "rmdir"
	default:
		return "unknown"
	}
}
