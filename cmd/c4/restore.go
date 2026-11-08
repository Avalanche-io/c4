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
	fs.help(restoreHelp)
	force := fs.boolFlag("force", 0, false, "Apply the restore (default is a dry run)")
	continueFlag := fs.stringFlag("continue", 'c', "", "Reuse guide: trust unchanged size+mtime from this c4m (racy-safe)")
	fs.parse(args)

	if len(fs.args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 restore [--force] [-c GUIDE.c4m] <snapshot-id|file.c4m> <dir>\n")
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
	// be servable from the store or from the destination itself, where
	// the plan only moves it. Missing content lists and refuses — nothing
	// is touched. Incomplete entries (null IDs) are declared; they will be
	// kept as found and own exit 2.
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
	// Content the destination already holds counts, by its ID: every file
	// is hashed here, never matched by size and time, since a move must
	// carry exactly the bytes the target names. Under --force this scan is
	// the pre-image, so the destination is hashed once.
	var destScan *c4m.Manifest
	var destScanStart time.Time
	if len(missing) > 0 && destExists {
		destScanStart = time.Now().UTC()
		destScan = scanDirectory(destArg, scan.ModeFull, false, false, nil, "", nil, time.Time{}, false)
		held := make(map[c4.ID]bool)
		for _, e := range destScan.Entries {
			if !e.IsDir() && e.Target == "" && !e.C4ID.IsNil() {
				held[e.C4ID] = true
			}
		}
		kept := missing[:0]
		for _, id := range missing {
			if !held[id] {
				kept = append(kept, id)
			}
		}
		missing = kept
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
		// The plan in c4m patch form, computed at the target's knowledge
		// level: line 1 is the destination's current ID at that level,
		// one line per differing entry, then the target ID. Matching
		// states print equal boundary IDs with no entry lines.
		// One scan serves the printed patch and the plan: every file
		// hashed unless -c names a guide.
		var current *c4m.Manifest
		scanStart := time.Now().UTC()
		if destExists {
			current = scanForCompare(destArg, level, *continueFlag)
		} else {
			current = c4m.NewManifest()
		}
		diff := c4m.PatchDiff(current, target)
		fmt.Println(diff.OldID)
		if !diff.IsEmpty() {
			c4m.NewEncoder(os.Stdout).Encode(diff.Patch)
		}
		fmt.Println(diff.NewID)

		plan := planRestore(s, target, destArg, current, scanStart, *continueFlag != "")
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
	preScanStart := preStart // when the pre-image's files were hashed
	switch {
	case destScan != nil:
		preManifest = destScan
		preScanStart = destScanStart
	case destExists:
		preManifest = scanDirectory(destArg, scan.ModeFull, false, false, nil, "", nil, time.Time{}, false)
	default:
		preManifest = c4m.NewManifest()
		if err := os.MkdirAll(destArg, 0755); err != nil {
			fatalf("Error creating %s: %v", destArg, err)
		}
	}
	storeManifestContent(preManifest, destArg, preStart)
	// The barrier stores what the destination held, so everything the
	// target names is in the store now: confirmed here, still before the
	// first destructive operation, so a store that could not take it
	// refuses with nothing touched.
	for id := range seen {
		if !s.Has(id) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Missing content for %d entries after storing the pre-image; nothing restored:\n", len(missing))
		for _, id := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", id)
		}
		os.Exit(1)
	}
	preID := preManifest.ComputeC4ID()
	fmt.Println(preID) // line 1: printed only after the journaled barrier above

	// Reconcile to the target, pulling content from the store. The plan
	// is made from the pre-image scan just taken (every file hashed):
	// its C4 IDs are the plan's current state, so the destination is
	// read once, and a file written since that scan is rehashed.
	plan := planRestore(s, target, destArg, preManifest, preScanStart, *continueFlag != "")
	r := restoreReconciler(s, preManifest, preScanStart, destArg, *continueFlag != "")
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

	// Verify at the target's knowledge level, field by field: the
	// as-built tree, with every field nulled that the target leaves null
	// at the same path, must recompute to the target's own root ID. A
	// null in the target never overwrote the path's own value (and a
	// null mode on a created file took the platform default), so those
	// fields legitimately differ from the target and are not compared.
	// Line 2 prints only when recomputation reproduces the target.
	built := projectToKnowledge(scanDirectory(destArg, level, false, false, nil, "", nil, time.Time{}, false), target)
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
// (optionally with /path descent by recorded entry name, expanded
// through stored directory records) or a .c4m description file.
// Restore targets are descriptions only.
func resolveRestoreTarget(s store.Store, arg string) *c4m.Manifest {
	if first := strings.SplitN(arg, "/", 2)[0]; looksLikeC4ID(first) {
		id, err := c4.Parse(first)
		if err != nil {
			fatalf("Error: invalid C4 ID: %v", err)
		}
		if !s.Has(id) {
			fatalf("Error: %s is not in the store", id)
		}
		id, _ = storeDescend(s, id, arg[len(first):])
		m := verifiedManifestFromStore(s, id)
		if m == nil {
			fatalf("Error: %s must resolve to a listing (extract files with c4 cat)", arg)
		}
		// A restore target must be the whole tree: restoring a partially
		// expanded one would remove every subtree the store lacks.
		m, missing := expandIfRecord(m, s)
		requireCompleteTree(arg, missing)
		validateRevertTarget(m)
		return m
	}
	if strings.HasSuffix(arg, ".c4m") {
		m, err := loadManifest(arg)
		if err != nil {
			fatalf("Error loading %s: %v", arg, err)
		}
		m, missing := expandIfRecord(m, s)
		requireCompleteTree(arg, missing)
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
// store and the bytes dest holds (per the current scan) as content
// sources. The planner hashes every file it compares unless trust was
// asked for with -c: until 2026-11-03 it trusted the target's size and
// mtime by default, so a same-size, same-mtime byte change planned no
// operation, and --force then failed its own verification with the
// changed bytes still in place.
func planRestore(s store.Store, target *c4m.Manifest, dest string, current *c4m.Manifest, scanStart time.Time, trust bool) *reconcile.Plan {
	if current == nil {
		current = c4m.NewManifest()
	}
	r := restoreReconciler(s, current, scanStart, dest, trust)
	plan, err := r.Plan(target, dest)
	if err != nil {
		fatalf("Error planning restore: %v", err)
	}
	return plan
}

// restoreReconciler plans and applies against current, the caller's own
// hashed scan of dest started at scanStart: the plan reuses its IDs for
// files that scan provably hashed and reads nothing twice.
func restoreReconciler(s store.Store, current *c4m.Manifest, scanStart time.Time, dest string, trust bool) *reconcile.Reconciler {
	return reconcile.New(
		reconcile.WithKnownState(current, scanStart),
		reconcile.WithTrustedMetadata(trust),
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

// projectToKnowledge returns a copy of built with every field nulled that
// target leaves null for the same path: mode, timestamp, size and C4 ID.
// Paths the target does not name are kept as built, so an extra file is
// still a difference. Directory aggregates the target leaves null are
// nulled too; ComputeC4ID then derives them the same way on both sides.
// Projection only ever removes knowledge.
func projectToKnowledge(built, target *c4m.Manifest) *c4m.Manifest {
	want := c4m.EntryPaths(target.Entries)
	out := built.Copy()
	for path, e := range c4m.EntryPaths(out.Entries) {
		t, ok := want[path]
		if !ok {
			continue
		}
		if t.Mode == 0 {
			e.Mode = 0
		}
		if t.Timestamp.Equal(c4m.NullTimestamp()) {
			e.Timestamp = c4m.NullTimestamp()
		}
		if t.Size < 0 {
			e.Size = -1
		}
		if t.C4ID.IsNil() {
			e.C4ID = c4.ID{}
		}
	}
	// Directory aggregates derive from their children's lines, which the
	// projection just changed (a nulled timestamp shortens the line and
	// changes the record), so every directory with listed children is
	// re-derived from the projected children, exactly as the normaliser
	// derives it.
	c4m.Normalize(out)
	return out
}
