package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/reconcile"
	"github.com/Avalanche-io/c4/scan"
	"github.com/Avalanche-io/c4/store"
)

func runPatch(args []string) {
	fs := newFlags("patch")
	n := fs.intFlag("number", 'n', 0, "Resolve to specific patch number (1-based)")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Output ergonomic form")
	quiet := fs.boolFlag("quiet", 'q', false, "Suppress stdout output (changeset)")
	storeFlag := fs.boolFlag("store", 's', false, "Also store content at removal time (prior state is captured by default)")
	reverseFlag := fs.boolFlag("reverse", 'r', false, "Revert dir to a stored prior state (changeset file or manifest ID)")
	dryRun := fs.boolFlag("dry-run", 0, false, "Show plan without making changes")
	sourceFlags := fs.stringArrayFlag("source", "Additional content source paths (repeatable)")
	noStore := fs.boolFlag("no-store", 0, false, "Skip prior-state capture and content storage")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode for directory arguments: s/m/f/c")
	fs.parse(args)

	if len(fs.args) == 0 {
		patchUsage()
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	// Reverse mode: c4 patch -r <changeset.c4m|manifest-id> dir/
	if *reverseFlag {
		if len(fs.args) != 2 {
			fmt.Fprintf(os.Stderr, "Usage: c4 patch -r <changeset.c4m|manifest-id> <dir>\n")
			os.Exit(1)
		}
		runPatchReverse(fs.args[0], fs.args[1], *storeFlag, *noStore, *dryRun, *quiet, *sourceFlags)
		return
	}

	switch len(fs.args) {
	case 1:
		runPatchSingle(fs.args[0], mode, *n, *ergonomic, *noStore)
	case 2:
		runPatchPair(fs.args[0], fs.args[1], mode, *ergonomic, *dryRun, *noStore, *storeFlag, *quiet, *sourceFlags)
	default:
		// 3+ args: multi-file chain resolution (existing behavior).
		runPatchChain(fs.args, *n, *ergonomic)
	}
}

func patchUsage() {
	fmt.Fprintf(os.Stderr, "Usage: c4 patch [flags] <target> [<dest>]\n\n")
	fmt.Fprintf(os.Stderr, "Apply target state: resolve diffs, scan directories, or reconcile filesystems.\n\n")
	fmt.Fprintf(os.Stderr, "Argument combinations:\n")
	fmt.Fprintf(os.Stderr, "  c4 patch <file.c4m>                Resolve patch chain → stdout\n")
	fmt.Fprintf(os.Stderr, "  c4 patch <dir>                     Scan dir, store content → c4m stdout\n")
	fmt.Fprintf(os.Stderr, "  c4 patch <file.c4m> <file.c4m>     Resolve chain → write dest c4m\n")
	fmt.Fprintf(os.Stderr, "  c4 patch <file.c4m> <dir>          Reconcile dir to match c4m\n")
	fmt.Fprintf(os.Stderr, "  c4 patch <dir> <file.c4m>          Scan dir, store, write c4m\n")
	fmt.Fprintf(os.Stderr, "  c4 patch <dir> <dir>               Reconcile dest dir to match source\n")
	fmt.Fprintf(os.Stderr, "  c4 patch <file.c4m>...             Multi-file chain resolution\n")
	fmt.Fprintf(os.Stderr, "\nReconcile forms store dest's prior state before applying\n")
	fmt.Fprintf(os.Stderr, "(revert: c4 patch -r <id> <dir>). --no-store opts out.\n")
}

// runPatchSingle handles single-argument patch.
func runPatchSingle(path string, mode scan.ScanMode, n int, ergonomic, noStore bool) {
	if isDirectory(path) {
		// Directory: scan, store, output c4m.
		shouldStore := !noStore && mode == scan.ModeFull
		m := scanDirectory(path, mode, false, shouldStore, nil, "", nil)
		outputManifest(m, ergonomic)
		return
	}

	// c4m file: chain resolution (original behavior).
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("Error reading %s: %v", path, err)
	}
	sections, err := c4m.DecodePatchChain(bytes.NewReader(data))
	if err != nil {
		fatalf("Error decoding %s: %v", path, err)
	}
	if len(sections) == 0 {
		fatalf("Error: no content found")
	}
	manifest := c4m.ResolvePatchChain(sections, n)
	outputManifest(manifest, ergonomic)
}

// runPatchPair handles two-argument patch with dispatch based on argument types.
func runPatchPair(target, dest string, mode scan.ScanMode, ergonomic, dryRun, noStore, storeRemovals, quiet bool, sources []string) {
	targetIsDir := isDirectory(target)
	destIsDir := isDirectory(dest)

	switch {
	case !targetIsDir && !destIsDir:
		runPatchC4mToC4m(target, dest, ergonomic)
	case !targetIsDir && destIsDir:
		runPatchC4mToDir(target, dest, mode, dryRun, noStore, storeRemovals, quiet, sources)
	case targetIsDir && !destIsDir:
		runPatchDirToC4m(target, dest, mode, noStore)
	default:
		runPatchDirToDir(target, dest, mode, dryRun, noStore, storeRemovals, quiet, sources)
	}
}

// runPatchC4mToC4m resolves the target manifest and writes it to dest.
func runPatchC4mToC4m(target, dest string, ergonomic bool) {
	m := resolveC4m(target)

	f, err := os.Create(dest)
	if err != nil {
		fatalf("Error creating %s: %v", dest, err)
	}
	defer f.Close()

	enc := c4m.NewEncoder(f)
	if ergonomic {
		enc.SetPretty(true)
	}
	if err := enc.Encode(m); err != nil {
		fatalf("Error writing %s: %v", dest, err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s\n", dest)
}

// runPatchC4mToDir reconciles a directory to match a c4m target state.
// Outputs the computed diff to stdout.
func runPatchC4mToDir(target, dirPath string, mode scan.ScanMode, dryRun, noStore, storeRemovals, quiet bool, sources []string) {
	targetManifest := resolveC4m(target)

	// Scan current state using target as a guide — only hash changed files.
	var currentManifest *c4m.Manifest
	if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
		currentManifest = guidedScan(dirPath, targetManifest, scan.ModeFull)
	} else {
		currentManifest = c4m.NewManifest()
	}

	s := reconcileStore(noStore)

	// Capture dest's prior state before anything is applied. Runs
	// before the diff is printed so the changeset's OldID matches the
	// stored root record.
	preID := maybeCapturePreState(s, currentManifest, targetManifest, dirPath, noStore, dryRun)

	// Output the diff to stdout (the changeset being applied).
	if !quiet {
		diff := c4m.PatchDiff(currentManifest, targetManifest)
		if !diff.IsEmpty() {
			fmt.Println(diff.OldID)
			c4m.NewEncoder(os.Stdout).Encode(diff.Patch)
			fmt.Println(diff.NewID)
		}
	}

	// Build content sources. Plan trusts size+mtime matches, consistent
	// with the guided scan above.
	var opts []reconcile.Option
	opts = append(opts, reconcile.WithTrustedMetadata(true))
	opts = append(opts, reconcile.WithSyncMode(store.SyncBatch))
	opts = append(opts, reconcile.WithSource(reconcile.NewDirSource(currentManifest, dirPath)))

	if s != nil {
		opts = append(opts, reconcile.WithSource(s))
	}
	if storeRemovals && s != nil {
		opts = append(opts, reconcile.WithStoreRemovals(s))
	}

	for _, src := range sources {
		if isDirectory(src) {
			srcManifest := resolveManifestOrDir(src, scan.ModeFull)
			opts = append(opts, reconcile.WithSource(reconcile.NewDirSource(srcManifest, src)))
		}
	}

	r := reconcile.New(opts...)
	plan, err := r.Plan(targetManifest, dirPath)
	if err != nil {
		fatalf("Error planning reconciliation: %v", err)
	}

	if len(plan.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "Missing content for %d entries:\n", len(plan.Missing))
		for _, id := range plan.Missing {
			fmt.Fprintf(os.Stderr, "  %s\n", id)
		}
		os.Exit(1)
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "%d operations planned\n", len(plan.Operations))
		for _, op := range plan.Operations {
			fmt.Fprintf(os.Stderr, "  %s %s\n", opName(op.Type), op.Path)
		}
		return
	}

	result, err := r.Apply(plan, dirPath)
	if err != nil {
		fatalf("Error applying reconciliation: %v", err)
	}
	syncStore(s)

	reportResult(dirPath, result)
	reportPreState(preID, dirPath)
}

// runPatchDirToC4m scans a directory, stores content, and writes a c4m file.
func runPatchDirToC4m(dirPath, destPath string, mode scan.ScanMode, noStore bool) {
	shouldStore := !noStore && mode == scan.ModeFull
	m := scanDirectory(dirPath, mode, false, shouldStore, nil, "", nil)

	f, err := os.Create(destPath)
	if err != nil {
		fatalf("Error creating %s: %v", destPath, err)
	}
	defer f.Close()

	enc := c4m.NewEncoder(f)
	if err := enc.Encode(m); err != nil {
		fatalf("Error writing %s: %v", destPath, err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %s\n", destPath)
}

// runPatchDirToDir scans source directory and reconciles dest to match.
// Outputs the computed diff to stdout.
func runPatchDirToDir(srcDir, destDir string, mode scan.ScanMode, dryRun, noStore, storeRemovals, quiet bool, sources []string) {
	shouldStore := !noStore && mode == scan.ModeFull
	targetManifest := scanDirectory(srcDir, mode, false, shouldStore, nil, "", nil)

	// Scan dest for diff output and content source.
	var destManifest *c4m.Manifest
	if info, err := os.Stat(destDir); err == nil && info.IsDir() {
		destManifest = resolveManifestOrDir(destDir, scan.ModeFull)
	} else {
		destManifest = c4m.NewManifest()
	}

	s := reconcileStore(noStore)

	// Capture dest's prior state before anything is applied.
	preID := maybeCapturePreState(s, destManifest, targetManifest, destDir, noStore, dryRun)

	// Output the diff to stdout.
	if !quiet {
		diff := c4m.PatchDiff(destManifest, targetManifest)
		if !diff.IsEmpty() {
			fmt.Println(diff.OldID)
			c4m.NewEncoder(os.Stdout).Encode(diff.Patch)
			fmt.Println(diff.NewID)
		}
	}

	var opts []reconcile.Option
	opts = append(opts, reconcile.WithTrustedMetadata(true))
	opts = append(opts, reconcile.WithSyncMode(store.SyncBatch))
	opts = append(opts, reconcile.WithSource(reconcile.NewDirSource(targetManifest, srcDir)))
	opts = append(opts, reconcile.WithSource(reconcile.NewDirSource(destManifest, destDir)))

	if s != nil {
		opts = append(opts, reconcile.WithSource(s))
	}
	if storeRemovals && s != nil {
		opts = append(opts, reconcile.WithStoreRemovals(s))
	}

	for _, src := range sources {
		if isDirectory(src) {
			srcManifest := resolveManifestOrDir(src, scan.ModeFull)
			opts = append(opts, reconcile.WithSource(reconcile.NewDirSource(srcManifest, src)))
		}
	}

	r := reconcile.New(opts...)
	plan, err := r.Plan(targetManifest, destDir)
	if err != nil {
		fatalf("Error planning reconciliation: %v", err)
	}

	if len(plan.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "Missing content for %d entries:\n", len(plan.Missing))
		for _, id := range plan.Missing {
			fmt.Fprintf(os.Stderr, "  %s\n", id)
		}
		os.Exit(1)
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "%d operations planned\n", len(plan.Operations))
		for _, op := range plan.Operations {
			fmt.Fprintf(os.Stderr, "  %s %s\n", opName(op.Type), op.Path)
		}
		return
	}

	result, err := r.Apply(plan, destDir)
	if err != nil {
		fatalf("Error applying reconciliation: %v", err)
	}
	syncStore(s)

	reportResult(destDir, result)
	reportPreState(preID, destDir)
}

// runPatchChain handles 3+ args: multi-file chain resolution (original behavior).
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

// runPatchReverse reverts a directory to a stored prior state. The first
// argument is either a changeset file — its first bare C4 ID (OldID) names
// the prior state — or the manifest ID printed by "prior state stored".
func runPatchReverse(source, dirPath string, storeRemovals, noStore bool, dryRun, quiet bool, sources []string) {
	if !isDirectory(dirPath) {
		fatalf("Error: %s is not a directory", dirPath)
	}

	s := reconcileStore(noStore)
	if s == nil {
		fatalf("Error: no content store configured (needed to load the prior state manifest)")
	}

	// Resolve the revert target: changeset file or stored manifest ID.
	var targetManifest *c4m.Manifest
	var sections []*c4m.PatchSection
	if _, err := os.Stat(source); err == nil {
		data, err := os.ReadFile(source)
		if err != nil {
			fatalf("Error reading %s: %v", source, err)
		}
		sections, err = c4m.DecodePatchChain(bytes.NewReader(data))
		if err != nil {
			fatalf("Error decoding %s: %v", source, err)
		}
		if len(sections) == 0 {
			fatalf("Error: changeset is empty")
		}
		// The OldID is the BaseID of the first section (or we compute it
		// from the section: no base reference means it IS the base).
		oldID := sections[0].BaseID
		if oldID.IsNil() {
			base := &c4m.Manifest{Version: "1.0", Entries: sections[0].Entries}
			oldID = base.ComputeC4ID()
		}
		targetManifest = manifestFromStore(s, oldID)
	} else if looksLikeC4ID(source) {
		id, err := c4.Parse(source)
		if err != nil {
			fatalf("Error: invalid C4 ID: %v", err)
		}
		targetManifest = manifestFromStore(s, id)
	} else {
		fatalf("Error: %s is neither a changeset file nor a C4 ID", source)
	}

	// A one-level root record expands into the full tree through the
	// stored per-directory records.
	targetManifest = expandIfRecord(targetManifest, s)
	validateRevertTarget(targetManifest)

	// Scan current directory state.
	currentManifest := resolveManifestOrDir(dirPath, scan.ModeFull)

	// Drift check (changeset form only): has the directory changed since
	// the forward patch? The changeset's NewID is the post-patch state.
	if sections != nil {
		changesetManifest := c4m.ResolvePatchChain(sections, 0)
		if currentManifest.ComputeC4ID() != changesetManifest.ComputeC4ID() {
			fmt.Fprintf(os.Stderr, "Warning: directory has changed since this patch was applied.\n")
			fmt.Fprintf(os.Stderr, "Reverting will also undo changes made after the original patch.\n")
		}
	}

	// Capture the current (pre-revert) state — the revert itself is
	// revertible.
	preID := maybeCapturePreState(s, currentManifest, targetManifest, dirPath, noStore, dryRun)

	// Output the reverse diff to stdout.
	if !quiet {
		diff := c4m.PatchDiff(currentManifest, targetManifest)
		if !diff.IsEmpty() {
			fmt.Println(diff.OldID)
			c4m.NewEncoder(os.Stdout).Encode(diff.Patch)
			fmt.Println(diff.NewID)
		}
	}

	// Build content sources and reconcile.
	var opts []reconcile.Option
	opts = append(opts, reconcile.WithTrustedMetadata(true))
	opts = append(opts, reconcile.WithSyncMode(store.SyncBatch))
	opts = append(opts, reconcile.WithSource(reconcile.NewDirSource(currentManifest, dirPath)))
	opts = append(opts, reconcile.WithSource(s))
	if storeRemovals {
		opts = append(opts, reconcile.WithStoreRemovals(s))
	}

	for _, src := range sources {
		if isDirectory(src) {
			srcManifest := resolveManifestOrDir(src, scan.ModeFull)
			opts = append(opts, reconcile.WithSource(reconcile.NewDirSource(srcManifest, src)))
		}
	}

	r := reconcile.New(opts...)
	plan, err := r.Plan(targetManifest, dirPath)
	if err != nil {
		fatalf("Error planning reconciliation: %v", err)
	}

	if len(plan.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "Missing content for %d entries:\n", len(plan.Missing))
		for _, id := range plan.Missing {
			fmt.Fprintf(os.Stderr, "  %s\n", id)
		}
		os.Exit(1)
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "%d operations planned\n", len(plan.Operations))
		for _, op := range plan.Operations {
			fmt.Fprintf(os.Stderr, "  %s %s\n", opName(op.Type), op.Path)
		}
		return
	}

	result, err := r.Apply(plan, dirPath)
	if err != nil {
		fatalf("Error applying reconciliation: %v", err)
	}
	syncStore(s)

	reportResult(dirPath, result)
	reportPreState(preID, dirPath)
}

// reconcileStore returns the store handle for a reconcile: the prompting
// ingest handle when the prior state will be captured, a silent open
// otherwise (--no-store must not prompt).
func reconcileStore(noStore bool) store.Store {
	if noStore {
		return openStoreOrNil()
	}
	return getOrSetupStore()
}

// maybeCapturePreState captures dest's prior state unless there is
// nothing to capture (empty dest), nothing will be destroyed (dry run),
// or the user opted out. Warns when no store is available to hold it.
func maybeCapturePreState(s store.Store, current, target *c4m.Manifest, dirPath string, noStore, dryRun bool) c4.ID {
	if noStore || dryRun || len(current.Entries) == 0 {
		return c4.ID{}
	}
	if s == nil {
		fmt.Fprintf(os.Stderr, "Warning: no content store configured — prior state not stored (use --no-store to silence)\n")
		return c4.ID{}
	}
	return capturePreState(s, current, target, dirPath)
}

// capturePreState makes a reconcile revertible before it destroys
// anything. It stores dest content that would vanish (removed or
// overwritten), each directory's one-level record, the root record, and
// the pre-state manifest text, then issues the durability barrier — the
// prior state is on stable storage before Apply mutates the directory.
// Returns the pre-state manifest's C4 ID.
//
// Capture writes ride the batch barrier like every other ingest:
// durability is one default behavior (D2). See design/safety-defaults.md.
func capturePreState(s store.Store, current, target *c4m.Manifest, dirPath string) c4.ID {
	// Content is vanishing if its ID appears nowhere among the target's
	// files: the post-patch tree cannot supply it at revert time.
	targetIDs := make(map[c4.ID]bool, len(target.Entries))
	for _, e := range target.Entries {
		if !e.IsDir() && !e.C4ID.IsNil() {
			targetIDs[e.C4ID] = true
		}
	}

	for path, e := range c4m.EntryPaths(current.Entries) {
		if e.IsDir() || e.Target != "" || e.C4ID.IsNil() ||
			targetIDs[e.C4ID] || s.Has(e.C4ID) {
			continue
		}
		if e.IsSequence {
			fmt.Fprintf(os.Stderr, "Warning: prior state: sequence %s not captured\n", path)
			continue
		}
		storeFileEntry(s, e, path, filepath.Join(dirPath, filepath.FromSlash(path)))
	}

	// Directory records make changeset-based -r work on nested trees.
	// Guided scans leave directory IDs nil — compute them deepest-first
	// so parent records embed child directory IDs.
	var dirs []*c4m.Entry
	for _, e := range current.Entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool { return dirs[i].Depth > dirs[j].Depth })
	for _, e := range dirs {
		if !e.C4ID.IsNil() && s.Has(e.C4ID) {
			continue
		}
		if id := storeDirectoryC4m(current, e, s); e.C4ID.IsNil() {
			e.C4ID = id
		}
	}

	id := storeManifestSelf(s, current)
	syncStore(s)
	return id
}

// validateRevertTarget refuses a prior state whose description is
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
			fatalf("Error: prior state incomplete: no stored record for directory %s (%s)", e.Name, e.C4ID)
		}
	}
}

// manifestFromStore loads and decodes a manifest stored by a prior
// state capture (or c4 id -s).
func manifestFromStore(s store.Store, id c4.ID) *c4m.Manifest {
	if !s.Has(id) {
		fatalf("Error: prior state manifest %s not found in store\n"+
			"Was the prior state stored? (default unless --no-store)", id)
	}
	rc, err := s.Open(id)
	if err != nil {
		fatalf("Error loading prior state manifest: %v", err)
	}
	defer rc.Close()
	m, err := c4m.NewDecoder(rc).Decode()
	if err != nil {
		fatalf("Error decoding prior state manifest: %v", err)
	}
	return m
}

// reportPreState prints the revert hint after a successful apply.
func reportPreState(id c4.ID, dirPath string) {
	if id.IsNil() {
		return
	}
	fmt.Fprintf(os.Stderr, "prior state stored: %s (revert: c4 patch -r %s %s)\n", id, id, dirPath)
}

// reportResult prints a reconciliation summary to stderr.
func reportResult(dirPath string, result *reconcile.Result) {
	fmt.Fprintf(os.Stderr, "Reconciled %s:", dirPath)
	if result.Created > 0 {
		fmt.Fprintf(os.Stderr, " %d created", result.Created)
	}
	if result.Moved > 0 {
		fmt.Fprintf(os.Stderr, " %d moved", result.Moved)
	}
	if result.Updated > 0 {
		fmt.Fprintf(os.Stderr, " %d updated", result.Updated)
	}
	if result.Removed > 0 {
		fmt.Fprintf(os.Stderr, " %d removed", result.Removed)
	}
	if result.Skipped > 0 {
		fmt.Fprintf(os.Stderr, " %d skipped", result.Skipped)
	}
	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, " %d errors", len(result.Errors))
	}
	fmt.Fprintln(os.Stderr)

	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "  error: %v\n", e)
	}
}
