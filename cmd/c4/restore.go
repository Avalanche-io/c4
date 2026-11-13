package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		fatalf("Error: restore needs a content store (its pre-image is the durable undo, stored before anything changes); %s", noStoreHint())
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
	var folded []string
	// A file entry with no C4 ID has no content to write: writing zeros of
	// its stated size would invent content. It is kept as found, named by
	// its full path, and left out of verification on both sides (decided
	// 2026-11-13 by the release lead; reversible).
	for path, e := range c4m.EntryPaths(target.Entries) {
		if !e.IsDir() && e.Target == "" && !e.IsSequence && e.C4ID.IsNil() {
			incomplete = append(incomplete, path)
		}
	}
	sort.Strings(incomplete)
	for _, e := range target.Entries {
		if e.IsDir() {
			continue
		}
		if e.Target != "" { // symlink: no store object needed
			continue
		}
		if e.IsSequence {
			// A folded entry names its members' ID list, not file bytes;
			// restore does not expand it. Writing the ID list under the
			// pattern's name and removing the real members was silent
			// corruption (release 1.0.x: 7afd4d8).
			folded = append(folded, e.Name)
			continue
		}
		if e.C4ID.IsNil() {
			continue // an incomplete entry, listed above
		}
		if seen[e.C4ID] {
			continue
		}
		seen[e.C4ID] = true
		if !s.Has(e.C4ID) {
			missing = append(missing, e.C4ID)
		}
	}
	if len(folded) > 0 {
		fmt.Fprintf(os.Stderr, "Cannot restore %d folded sequence entries; nothing restored (restore a description made without -S):\n", len(folded))
		for _, name := range folded {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		os.Exit(1)
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
		// then one line per differing entry. Nothing follows them: the
		// target's ID is the resolved state's own ID, and a c4m stream
		// never ends with its own ID (2026-11-08). Matching states print
		// the boundary ID alone. One scan serves the printed patch and
		// the plan: every file hashed unless -c names a guide.
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

		plan := planRestore(s, target, destArg, current, scanStart, current, *continueFlag != "", true)
		fmt.Fprintf(os.Stderr, "dry run: %d operations planned for %s (use --force to apply)\n",
			len(plan.Operations), destArg)
		for _, op := range plan.Operations {
			fmt.Fprintf(os.Stderr, "  %s %s\n", opName(op.Type), op.Path)
		}
		if n := reportRefused(plan.Refused); n > 0 {
			fmt.Fprintf(os.Stderr, "dry run: %d entries refused (names this destination cannot hold); the rest is planned\n", n)
		}
		reportCollisionCandidates(plan)
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
	// With -c the plan is made from the same view the dry run plans from:
	// the guide-trusted scan, which reuses a guide entry's ID when size
	// and mtime match under the racy rule. --force used to plan from the
	// fully hashed pre-image instead, so the dry run planned nothing and
	// --force then rewrote the file the client had said to trust. The
	// pre-image is still hashed in full (it is the durable backup and must
	// hold the real bytes), and content taken from the destination still
	// comes from it, so a trusted ID never vouches for bytes copied to
	// another path.
	trust := *continueFlag != ""
	planCurrent, planStart := preManifest, preScanStart
	if trust && destExists {
		planStart = time.Now().UTC()
		planCurrent = scanForCompare(destArg, level, *continueFlag)
	}
	plan := planRestore(s, target, destArg, planCurrent, planStart, preManifest, trust, false)
	r := restoreReconciler(s, planCurrent, planStart, preManifest, destArg, trust)
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
	// Verification is made at the same stated trust: with -c, a file the
	// guide vouches for (size and mtime unchanged, racy-safe) is the
	// guide's, as it was for the plan; every file this restore wrote is
	// newer than the guide and is hashed.
	var builtScan *c4m.Manifest
	if trust {
		builtScan = scanForCompare(destArg, level, *continueFlag)
	} else {
		builtScan = scanDirectory(destArg, level, false, false, nil, "", nil, time.Time{}, false)
	}
	verifyTarget := linkTimesAsBuilt(target, builtScan, destArg, result.LinkTimesUnset)
	// Entries this platform cannot hold were refused, not written: the
	// rest is verified exactly, on both sides without them, and the
	// refused set is named in full so "verified" never reads as
	// "complete".
	refusedCount := reportRefused(plan.Refused)
	reportCollisionCandidates(plan)
	if refusedCount > 0 {
		verifyTarget = withoutRefused(verifyTarget, plan.Refused)
		builtScan = withoutRefused(builtScan, plan.Refused)
	}
	// Declared omissions were never written: compare what was. Through
	// 2026-11-13 they stayed in the comparison, so a restore that did all
	// it could failed here, exit 3, with a "re-run" hint no re-run could
	// satisfy.
	if len(incomplete) > 0 {
		verifyTarget = withoutPaths(verifyTarget, incomplete)
		builtScan = withoutPaths(builtScan, incomplete)
	}
	built := projectToKnowledge(builtScan, verifyTarget)
	builtID := built.ComputeC4ID()
	targetID := verifyTarget.ComputeC4ID()
	if builtID != targetID {
		fmt.Fprintf(os.Stderr, "verification failed: as-built %s != target %s\nre-run the restore; the pre-image %s is durable\n",
			builtID, targetID, preID)
		os.Exit(3)
	}
	fmt.Println(builtID) // line 2: the as-built ID, verified

	if refusedCount > 0 {
		fmt.Fprintf(os.Stderr, "partial: %d entries refused (names this destination cannot hold); the rest is written and verified\n", refusedCount)
	}
	if len(incomplete) > 0 || refusedCount > 0 {
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
		// A content-level entry carries the content projection's mode:
		// null, or lrwxrwxrwx for a symlink (scan.ContentMode).
		if e.Mode != scan.ContentMode(e.Mode) || !e.Timestamp.Equal(c4m.NullTimestamp()) {
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
func planRestore(s store.Store, target *c4m.Manifest, dest string, current *c4m.Manifest, scanStart time.Time, source *c4m.Manifest, trust, dryRun bool) *reconcile.Plan {
	if current == nil {
		current = c4m.NewManifest()
	}
	if source == nil {
		source = current
	}
	r := restoreReconciler(s, current, scanStart, source, dest, trust, reconcile.WithDryRun(dryRun))
	plan, err := r.Plan(target, dest)
	if err != nil {
		fatalf("Error planning restore: %v", err)
	}
	// Content the planner cannot materialise (a folded sequence entry, which
	// names an ID list rather than file bytes) refuses before anything is
	// applied. An incomplete plan used to apply as zero operations and then
	// fail its own verification with exit 3.
	if !plan.IsComplete() {
		fmt.Fprintf(os.Stderr, "Cannot restore %d entries; nothing restored:\n", len(plan.Missing))
		for _, id := range plan.Missing {
			fmt.Fprintf(os.Stderr, "  %s\n", id)
		}
		os.Exit(1)
	}
	return plan
}

// restoreReconciler plans and applies against current, the caller's own
// hashed scan of dest started at scanStart: the plan reuses its IDs for
// files that scan provably hashed and reads nothing twice.
// source is the hashed scan whose IDs name the destination's bytes when
// they are copied or moved: the same as current unless current trusts a
// guide.
func restoreReconciler(s store.Store, current *c4m.Manifest, scanStart time.Time, source *c4m.Manifest, dest string, trust bool, extra ...reconcile.Option) *reconcile.Reconciler {
	opts := []reconcile.Option{
		reconcile.WithKnownState(current, scanStart),
		reconcile.WithTrustedMetadata(trust),
		reconcile.WithSyncMode(store.SyncBatch),
		reconcile.WithSource(reconcile.NewDirSource(source, dest)),
		reconcile.WithSource(s),
	}
	return reconcile.New(append(opts, extra...)...)
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
// linkTimesAsBuilt returns the target to verify against when this platform
// could not set some symlinks' own times (reconcile.Result.LinkTimesUnset;
// os.Chtimes would follow a link and change its target instead). Each such
// link keeps the time it was created with: it is named on stderr, and the
// verification target takes that link's time from the as-built tree, so
// verification checks everything that was set and says which times were
// not, rather than failing on them or skipping them silently. With nothing
// unset it returns target unchanged.
func linkTimesAsBuilt(target, built *c4m.Manifest, dest string, unset []string) *c4m.Manifest {
	if len(unset) == 0 {
		return target
	}
	out := target.Copy()
	want := c4m.EntryPaths(out.Entries)
	have := c4m.EntryPaths(built.Entries)
	for _, abs := range unset {
		rel, err := filepath.Rel(dest, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		t, b := want[rel], have[rel]
		if t == nil || b == nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "restore: %s: this platform cannot set a symlink's own time; it keeps its creation time %s (target %s), and is verified at that time\n",
			rel, b.Timestamp.UTC().Format(time.RFC3339), t.Timestamp.UTC().Format(time.RFC3339))
		t.Timestamp = b.Timestamp
	}
	c4m.Normalize(out)
	return out
}

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

// reportCollisionCandidates names, on stderr, what a dry run could not
// measure: sibling names that differ only in case (a hint; a dry run
// writes nothing, so it makes no probe), and any folder a real run could
// not probe, whose names are then written as named.
func reportCollisionCandidates(plan *reconcile.Plan) {
	for _, c := range plan.MayCollide {
		fmt.Fprintf(os.Stderr, "may collide: %s with %s (not measured)\n", c.Path, c.With)
	}
	if len(plan.MayCollide) > 0 {
		fmt.Fprintf(os.Stderr, "a real run measures every folder, and also catches folds this hint cannot predict (Unicode forms)\n")
	}
	for _, c := range plan.Unmeasured {
		dir := c.Path
		if dir == "" {
			dir = "the destination"
		}
		fmt.Fprintf(os.Stderr, "not measured: %s: %s; its names are written as named\n", dir, c.Err)
	}
}

// reportRefused names, on stderr, every target entry the destination cannot
// hold (a name the platform refuses, or one the folder folds onto a
// sibling's): the path, why, and for a directory how many entries went with it.
// It returns the number of entries refused.
func reportRefused(refused []reconcile.Refusal) int {
	n := 0
	for _, r := range refused {
		n += r.Entries
		if r.Entries > 1 {
			fmt.Fprintf(os.Stderr, "refused: %s (%d entries): %s\n", r.Path, r.Entries, r.Reason)
		} else {
			fmt.Fprintf(os.Stderr, "refused: %s: %s\n", r.Path, r.Reason)
		}
	}
	return n
}

// withoutRefused returns m without the refused entries and their
// subtrees, with every directory above one re-derived from what remains.
// Verification applies it to both the target and the as-built tree, so
// the rest is compared exactly and the refused entries are not; the
// refused set is reported in full beside it.
func withoutRefused(m *c4m.Manifest, refused []reconcile.Refusal) *c4m.Manifest {
	paths := make([]string, len(refused))
	for i, r := range refused {
		paths[i] = r.Path
	}
	return withoutPaths(m, paths)
}

// withoutPaths drops the entries at the given paths (a directory path, with
// its trailing slash, takes its subtree), nulls the size and C4 ID of every
// directory above them, which depend on what was dropped, and re-derives
// the rest. Applied to the as-built tree and to the target alike, it
// compares exactly what a restore could write.
func withoutPaths(m *c4m.Manifest, omitted []string) *c4m.Manifest {
	out := m.Copy()
	paths := c4m.EntryPaths(out.Entries)
	drop := make(map[*c4m.Entry]bool)
	ancestors := make(map[string]bool)
	for _, path := range omitted {
		for p, e := range paths {
			if p == path || (strings.HasSuffix(path, "/") && strings.HasPrefix(p, path)) {
				drop[e] = true
			}
		}
		parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
		for i := 1; i < len(parts); i++ {
			ancestors[strings.Join(parts[:i], "/")+"/"] = true
		}
	}
	for p, e := range paths {
		if ancestors[p] && !drop[e] {
			e.C4ID, e.Size = c4.ID{}, -1
		}
	}
	kept := out.Entries[:0]
	for _, e := range out.Entries {
		if !drop[e] {
			kept = append(kept, e)
		}
	}
	out.Entries = kept
	c4m.Normalize(out)
	return out
}
