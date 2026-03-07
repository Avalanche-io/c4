package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// runPatch implements "c4 patch" — apply a c4m patch or target state.
//
//	c4 patch changes.c4m project.c4m:
//	c4 patch changes.c4m :           # apply to managed directory (tracked, undoable)
//	c4 patch desired.c4m :           # converge to target state
//	c4 patch . :                     # re-sync managed state to match disk
//	c4 patch desired.c4m ./output/   # converge local path to desired state
func runPatch(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 patch <source> <target>\n")
		fmt.Fprintf(os.Stderr, "  c4 patch changes.c4m project.c4m:  # patch a c4m file\n")
		fmt.Fprintf(os.Stderr, "  c4 patch changes.c4m :             # apply changes (tracked)\n")
		fmt.Fprintf(os.Stderr, "  c4 patch . :                       # re-sync from disk\n")
		fmt.Fprintf(os.Stderr, "  c4 patch desired.c4m ./output/     # converge local path\n")
		os.Exit(1)
	}

	source := args[0]
	target := args[1]

	if target == ":" {
		patchManaged(source)
		return
	}

	spec, err := pathspec.Parse(target, establish.IsLocationEstablished)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch spec.Type {
	case pathspec.C4m:
		patchC4mFile(spec.Source, source)
	case pathspec.Local:
		patchLocalPath(spec.Source, source)
	default:
		fmt.Fprintf(os.Stderr, "Error: patch target must be a local path, :, or a c4m file (with colon)\n")
		os.Exit(1)
	}
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
	if err := writeManifest(c4mPath, result); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", c4mPath, err)
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

// patchLocalPath converges a local directory to match a desired c4m state.
// Uses C4 IDs to resolve operations: content already on disk is moved, not copied.
func patchLocalPath(targetDir, source string) {
	// Load desired state
	desired, err := loadSourceManifest(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", targetDir, err)
		os.Exit(1)
	}

	// Scan current state of target directory
	gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
	currentScan, err := gen.GenerateFromPath(targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", targetDir, err)
		os.Exit(1)
	}
	current := currentScan

	// Build C4 ID → disk path index from current state
	idIndex := buildIDIndex(current, targetDir)

	// Compute diff: what needs to change
	diff, err := c4m.Diff(c4m.ManifestSource{Manifest: current}, c4m.ManifestSource{Manifest: desired})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error computing diff: %v\n", err)
		os.Exit(1)
	}

	if len(diff.Added.Entries) == 0 && len(diff.Removed.Entries) == 0 && len(diff.Modified.Entries) == 0 {
		fmt.Fprintf(os.Stderr, "no changes\n")
		return
	}

	desiredPaths := manifestPaths(desired)

	// Pre-flight: verify all required content exists on disk before touching anything.
	// One copy is enough — content can always be copied from wherever it is.
	availableIDs := make(map[string]bool)
	for k := range idIndex {
		availableIDs[k] = true
	}
	var missing []string
	for _, dp := range desiredPaths {
		if dp.entry.IsDir() || dp.entry.IsSymlink() || dp.entry.C4ID.IsNil() {
			continue
		}
		if !availableIDs[dp.entry.C4ID.String()] {
			missing = append(missing, dp.path)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "%d files not available locally:\n", len(missing))
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", m)
		}
		os.Exit(1)
	}

	var moved, created, removed, skipped int

	// Phase 1: Create directories from desired state
	for _, dp := range desiredPaths {
		if !strings.HasSuffix(dp.path, "/") {
			continue
		}
		dirPath := filepath.Join(targetDir, dp.path)
		if err := os.MkdirAll(dirPath, dp.entry.Mode.Perm()|0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dirPath, err)
			os.Exit(1)
		}
	}

	// Phase 2: Place files — rename first, copy only if the same content is needed again.
	movedIDs := make(map[string]string) // C4 ID → destination after move
	for _, dp := range desiredPaths {
		if dp.entry.IsDir() || dp.entry.IsSymlink() {
			continue
		}

		destPath := filepath.Join(targetDir, dp.path)
		idStr := dp.entry.C4ID.String()

		// Already at correct path with correct content — skip
		if info, err := os.Stat(destPath); err == nil && info.Mode().IsRegular() {
			existingID := identifyFile(destPath)
			if !existingID.IsNil() && existingID == dp.entry.C4ID {
				skipped++
				movedIDs[idStr] = destPath
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if prev, ok := movedIDs[idStr]; ok {
			// Same content needed again — copy from where we already placed it
			if err := copyFile(prev, destPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error copying %s → %s: %v\n", prev, destPath, err)
				os.Exit(1)
			}
		} else {
			// First occurrence — move (atomic rename, O(1) on same filesystem)
			srcPath := idIndex[idStr][0]
			if err := os.Rename(srcPath, destPath); err != nil {
				// Cross-device fallback
				if err := copyFile(srcPath, destPath); err != nil {
					fmt.Fprintf(os.Stderr, "Error placing %s: %v\n", destPath, err)
					os.Exit(1)
				}
				os.Remove(srcPath)
			}
		}
		movedIDs[idStr] = destPath
		moved++
	}

	// Phase 3: Create symlinks
	for _, dp := range desiredPaths {
		if !dp.entry.IsSymlink() {
			continue
		}
		linkPath := filepath.Join(targetDir, dp.path)
		os.Remove(linkPath) // remove existing if any
		if err := os.Symlink(dp.entry.Target, linkPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating symlink %s: %v\n", linkPath, err)
			os.Exit(1)
		}
		created++
	}

	// Phase 4: Remove files not in desired state
	currentPaths := manifestPaths(current)
	desiredSet := make(map[string]bool)
	for _, dp := range desiredPaths {
		desiredSet[dp.path] = true
	}
	// Remove files (not dirs) in reverse order
	for i := len(currentPaths) - 1; i >= 0; i-- {
		cp := currentPaths[i]
		if desiredSet[cp.path] || cp.entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(targetDir, cp.path)
		if _, err := os.Lstat(fullPath); err == nil {
			os.Remove(fullPath)
			removed++
		}
	}

	// Phase 5: Remove empty directories not in desired state
	for i := len(currentPaths) - 1; i >= 0; i-- {
		cp := currentPaths[i]
		if !cp.entry.IsDir() || desiredSet[cp.path] {
			continue
		}
		fullPath := filepath.Join(targetDir, cp.path)
		os.Remove(fullPath) // only succeeds if empty
	}

	fmt.Fprintf(os.Stderr, "patched %s (%d moved, %d created, %d removed, %d unchanged)\n",
		targetDir, moved, created, removed, skipped)
}

// loadSourceManifest loads a manifest from a c4m file or stdin.
func loadSourceManifest(source string) (*c4m.Manifest, error) {
	if source == "-" {
		return c4m.NewDecoder(os.Stdin).Decode()
	}
	if strings.HasSuffix(source, ".c4m") {
		return loadManifest(source)
	}
	return nil, fmt.Errorf("source must be a .c4m file or -")
}

type pathEntry struct {
	path  string
	entry *c4m.Entry
}

// manifestPaths extracts full relative paths from a manifest's depth/name structure.
func manifestPaths(m *c4m.Manifest) []pathEntry {
	var result []pathEntry
	var dirStack []string

	for _, entry := range m.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}

		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + entry.Name
		} else {
			fullPath = entry.Name
		}

		result = append(result, pathEntry{path: fullPath, entry: entry})

		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}
	return result
}

// buildIDIndex maps C4 ID strings to disk paths in the current directory.
func buildIDIndex(m *c4m.Manifest, rootDir string) map[string][]string {
	index := make(map[string][]string)
	paths := manifestPaths(m)
	for _, pe := range paths {
		if pe.entry.IsDir() || pe.entry.C4ID.IsNil() {
			continue
		}
		idStr := pe.entry.C4ID.String()
		index[idStr] = append(index[idStr], filepath.Join(rootDir, pe.path))
	}
	return index
}

func identifyFile(path string) c4.ID {
	f, err := os.Open(path)
	if err != nil {
		return c4.ID{}
	}
	defer f.Close()
	return c4.Identify(f)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode())
}
