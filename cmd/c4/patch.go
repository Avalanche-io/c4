package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
	"github.com/Avalanche-io/c4/reconcile"
	"github.com/Avalanche-io/c4/store"
)

func runPatch(args []string) {
	fs := newFlags("patch")
	n := fs.intFlag("number", 'n', 0, "Resolve to specific patch number (1-based)")
	ergonomic := fs.boolFlag("ergonomic", 'e', false, "Output ergonomic form")
	storeFlag := fs.boolFlag("store", 's', false, "Store content that would be removed")
	reverseFlag := fs.boolFlag("reverse", 'r', false, "Reverse: revert to pre-patch state using stored manifest")
	dryRun := fs.boolFlag("dry-run", 0, false, "Show plan without making changes")
	sourceFlags := fs.stringArrayFlag("source", "Additional content source paths (repeatable)")
	noStore := fs.boolFlag("no-store", 0, false, "Suppress content storage")
	modeFlag := fs.stringFlag("mode", 'm', "f", "Scan mode for directory arguments: s/m/f")
	fs.parse(args)

	if len(fs.args) == 0 {
		patchUsage()
		os.Exit(1)
	}

	mode, err := scan.ParseScanMode(*modeFlag)
	if err != nil {
		fatalf("Error: %v", err)
	}

	// Reverse mode: c4 patch -r changeset.c4m dir/
	if *reverseFlag {
		if len(fs.args) != 2 {
			fmt.Fprintf(os.Stderr, "Usage: c4 patch -r [-s] <changeset.c4m> <dir>\n")
			os.Exit(1)
		}
		runPatchReverse(fs.args[0], fs.args[1], *storeFlag, *dryRun, *sourceFlags)
		return
	}

	switch len(fs.args) {
	case 1:
		runPatchSingle(fs.args[0], mode, *n, *ergonomic, *noStore)
	case 2:
		runPatchPair(fs.args[0], fs.args[1], mode, *ergonomic, *dryRun, *noStore, *storeFlag, *sourceFlags)
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
func runPatchPair(target, dest string, mode scan.ScanMode, ergonomic, dryRun, noStore, storeRemovals bool, sources []string) {
	targetIsDir := isDirectory(target)
	destIsDir := isDirectory(dest)

	switch {
	case !targetIsDir && !destIsDir:
		runPatchC4mToC4m(target, dest, ergonomic)
	case !targetIsDir && destIsDir:
		runPatchC4mToDir(target, dest, mode, dryRun, storeRemovals, sources)
	case targetIsDir && !destIsDir:
		runPatchDirToC4m(target, dest, mode, noStore)
	default:
		runPatchDirToDir(target, dest, mode, dryRun, noStore, storeRemovals, sources)
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
func runPatchC4mToDir(target, dirPath string, mode scan.ScanMode, dryRun, storeRemovals bool, sources []string) {
	targetManifest := resolveC4m(target)

	// Scan current state using target as a guide — only hash changed files.
	var currentManifest *c4m.Manifest
	if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
		currentManifest = guidedScan(dirPath, targetManifest, scan.ModeFull)
	} else {
		currentManifest = c4m.NewManifest()
	}

	// Output the diff to stdout (the changeset being applied).
	diff := c4m.PatchDiff(currentManifest, targetManifest)
	if !diff.IsEmpty() {
		fmt.Println(diff.OldID)
		c4m.NewEncoder(os.Stdout).Encode(diff.Patch)
		fmt.Println(diff.NewID)
	}

	// Store the pre-patch manifest if -s is set (enables -r reversal later).
	s, _ := store.OpenStore()
	if storeRemovals && s != nil {
		storeManifestAsContent(currentManifest, s)
	}

	// Build content sources.
	var opts []reconcile.Option
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

	reportResult(dirPath, result)
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
func runPatchDirToDir(srcDir, destDir string, mode scan.ScanMode, dryRun, noStore, storeRemovals bool, sources []string) {
	shouldStore := !noStore && mode == scan.ModeFull
	targetManifest := scanDirectory(srcDir, mode, false, shouldStore, nil, "", nil)

	// Scan dest for diff output and content source.
	var destManifest *c4m.Manifest
	if info, err := os.Stat(destDir); err == nil && info.IsDir() {
		destManifest = resolveManifestOrDir(destDir, scan.ModeFull)
	} else {
		destManifest = c4m.NewManifest()
	}

	// Output the diff to stdout.
	diff := c4m.PatchDiff(destManifest, targetManifest)
	if !diff.IsEmpty() {
		fmt.Println(diff.OldID)
		c4m.NewEncoder(os.Stdout).Encode(diff.Patch)
		fmt.Println(diff.NewID)
	}

	// Store the pre-patch manifest if -s is set.
	s, _ := store.OpenStore()
	if storeRemovals && s != nil {
		storeManifestAsContent(destManifest, s)
	}

	var opts []reconcile.Option
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

	reportResult(destDir, result)
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

// runPatchReverse reverts a directory to the pre-patch state using a stored manifest.
// The changeset's first bare C4 ID (OldID) identifies the pre-patch manifest
// which must be in the content store (stored by a prior -s operation).
func runPatchReverse(changesetPath, dirPath string, storeRemovals bool, dryRun bool, sources []string) {
	if !isDirectory(dirPath) {
		fatalf("Error: %s is not a directory", dirPath)
	}

	// Read the changeset to extract OldID (first bare C4 ID).
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

	// The OldID is the BaseID of the first section (or we compute it from the section).
	oldID := sections[0].BaseID
	if oldID.IsNil() {
		// First section has no base reference — it IS the base. Compute its ID.
		base := &c4m.Manifest{Version: "1.0", Entries: sections[0].Entries}
		oldID = base.ComputeC4ID()
	}

	// Load the pre-patch manifest from the store.
	s, _ := store.OpenStore()
	if s == nil {
		fatalf("Error: no content store configured (needed to load pre-patch manifest)")
	}
	if !s.Has(oldID) {
		fatalf("Error: pre-patch manifest %s not found in store\n"+
			"Was the original patch run with -s?", oldID)
	}

	rc, err := s.Open(oldID)
	if err != nil {
		fatalf("Error loading pre-patch manifest: %v", err)
	}
	targetManifest, err := c4m.NewDecoder(rc).Decode()
	rc.Close()
	if err != nil {
		fatalf("Error decoding pre-patch manifest: %v", err)
	}

	// Scan current directory state.
	currentManifest := resolveManifestOrDir(dirPath, scan.ModeFull)

	// Check for drift: has the directory changed since the forward patch?
	currentID := currentManifest.ComputeC4ID()
	// The changeset's NewID is the post-patch state. If current differs, warn.
	changesetManifest := c4m.ResolvePatchChain(sections, 0)
	expectedID := changesetManifest.ComputeC4ID()
	if currentID != expectedID {
		fmt.Fprintf(os.Stderr, "Warning: directory has changed since this patch was applied.\n")
		fmt.Fprintf(os.Stderr, "Reverting will also undo changes made after the original patch.\n")
		fmt.Fprintf(os.Stderr, "Use -s and redirect stdout to capture the reverse changeset.\n")
	}

	// Output the reverse diff to stdout.
	diff := c4m.PatchDiff(currentManifest, targetManifest)
	if !diff.IsEmpty() {
		fmt.Println(diff.OldID)
		c4m.NewEncoder(os.Stdout).Encode(diff.Patch)
		fmt.Println(diff.NewID)
	}

	// Store current state manifest if -s is set (for re-reversal).
	if storeRemovals {
		storeManifestAsContent(currentManifest, s)
	}

	// Build content sources and reconcile.
	var opts []reconcile.Option
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

	reportResult(dirPath, result)
}

// storeManifestAsContent stores a manifest's canonical c4m as content in the store.
// This enables future -r reversal by storing the pre-patch state keyed by its C4 ID.
func storeManifestAsContent(m *c4m.Manifest, s store.Store) {
	data, err := c4m.Marshal(m)
	if err != nil {
		return
	}
	if _, err := s.Put(bytes.NewReader(data)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store pre-patch manifest: %v\n", err)
	}
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
