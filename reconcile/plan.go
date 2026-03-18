package reconcile

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// Plan compares a target manifest against the directory at dirPath and returns
// an ordered operation list. If required content is missing from all sources,
// Plan.Missing is populated and Operations is empty.
func (r *Reconciler) Plan(target *c4m.Manifest, dirPath string) (*Plan, error) {
	dirPath, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, err
	}

	// 1. Build target path map from manifest entries.
	targetPaths := c4m.EntryPaths(target.Entries)

	// 2. Scan current directory state.
	currentFiles := make(map[string]os.FileInfo) // relative path -> info
	currentIDs := make(map[string]c4.ID)          // relative path -> C4 ID
	idToCurrentPaths := make(map[c4.ID][]string)  // C4 ID -> relative paths

	if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
		filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			rel, err := filepath.Rel(dirPath, path)
			if err != nil || rel == "." {
				return nil
			}
			// Normalize to forward slashes with trailing slash for dirs.
			rel = filepath.ToSlash(rel)
			if info.IsDir() {
				rel += "/"
			}
			currentFiles[rel] = info

			// Compute C4 ID for regular files.
			if info.Mode().IsRegular() && info.Size() >= 0 {
				f, ferr := os.Open(path)
				if ferr == nil {
					id := c4.Identify(f)
					f.Close()
					currentIDs[rel] = id
					idToCurrentPaths[id] = append(idToCurrentPaths[id], rel)
				}
			}
			return nil
		})
	}

	// 3. Classify each path.
	var (
		mkdirs   []Operation
		creates  []Operation
		moves    []Operation
		symlinks []Operation
		chmods   []Operation
		chtimes  []Operation
		removes  []Operation
		rmdirs   []Operation
	)

	// Track which target C4 IDs need content for creates.
	needsContent := make(map[c4.ID]bool)
	// Track which current paths are accounted for by the target.
	targetAccountedFor := make(map[string]bool)

	// Process target entries.
	for relPath, entry := range targetPaths {
		absPath := filepath.Join(dirPath, filepath.FromSlash(relPath))
		targetAccountedFor[relPath] = true

		if entry.IsDir() {
			if _, exists := currentFiles[relPath]; !exists {
				mkdirs = append(mkdirs, Operation{
					Type:  OpMkdir,
					Path:  absPath,
					Entry: entry,
				})
			}
			continue
		}

		if entry.IsSymlink() {
			curInfo, exists := currentFiles[relPath]
			if exists && curInfo.Mode()&os.ModeSymlink != 0 {
				// Check if symlink target matches.
				curTarget, lerr := os.Readlink(filepath.Join(dirPath, filepath.FromSlash(relPath)))
				if lerr == nil && curTarget == entry.Target {
					continue // already correct
				}
			}
			symlinks = append(symlinks, Operation{
				Type:  OpSymlink,
				Path:  absPath,
				Entry: entry,
			})
			continue
		}

		// Regular file.
		curInfo, exists := currentFiles[relPath]
		if exists {
			curID := currentIDs[relPath]
			sameContent := !entry.C4ID.IsNil() && curID == entry.C4ID

			if sameContent {
				// Content matches; check metadata.
				// Windows doesn't support Unix permissions; skip chmod there.
			needChmod := runtime.GOOS != "windows" &&
				entry.Mode != 0 && curInfo.Mode().Perm() != entry.Mode.Perm()
				needChtimes := !entry.Timestamp.Equal(c4m.NullTimestamp()) &&
					!curInfo.ModTime().UTC().Equal(entry.Timestamp.UTC())

				if needChmod {
					chmods = append(chmods, Operation{
						Type:  OpChmod,
						Path:  absPath,
						Entry: entry,
					})
				}
				if needChtimes {
					chtimes = append(chtimes, Operation{
						Type:  OpChtimes,
						Path:  absPath,
						Entry: entry,
					})
				}
				continue // skip, content is already right
			}
		}

		// Need to create or overwrite.
		if entry.C4ID.IsNil() {
			continue // cannot create without a C4 ID
		}
		needsContent[entry.C4ID] = true
		creates = append(creates, Operation{
			Type:      OpCreate,
			Path:      absPath,
			Entry:     entry,
			ContentID: entry.C4ID,
		})
	}

	// 4. Identify removals: paths in current but not in target.
	for relPath, info := range currentFiles {
		if targetAccountedFor[relPath] {
			continue
		}
		absPath := filepath.Join(dirPath, filepath.FromSlash(relPath))
		if info.IsDir() {
			rmdirs = append(rmdirs, Operation{
				Type: OpRmdir,
				Path: absPath,
			})
		} else {
			removes = append(removes, Operation{
				Type:      OpRemove,
				Path:      absPath,
				ContentID: currentIDs[relPath],
			})
		}
	}

	// 5. Move optimization: if a file is being removed and the same C4 ID
	//    is needed by a create, replace with a move.
	removeByID := make(map[c4.ID]int) // C4 ID -> index in removes
	for i, op := range removes {
		rel, _ := filepath.Rel(dirPath, op.Path)
		rel = filepath.ToSlash(rel)
		if id, ok := currentIDs[rel]; ok {
			removeByID[id] = i
		}
	}

	var filteredCreates []Operation
	for _, op := range creates {
		if rmIdx, ok := removeByID[op.ContentID]; ok {
			moves = append(moves, Operation{
				Type:      OpMove,
				Path:      op.Path,
				SrcPath:   removes[rmIdx].Path,
				Entry:     op.Entry,
				ContentID: op.ContentID,
			})
			delete(needsContent, op.ContentID)
			// Mark the remove as consumed.
			removes[rmIdx].Type = -1 // sentinel
		} else {
			filteredCreates = append(filteredCreates, op)
		}
	}
	creates = filteredCreates

	// Also check content sources for remaining creates.
	// If content can also be found in idToCurrentPaths (file exists elsewhere
	// in the current tree but is NOT being removed), we can still source it.

	// Filter removes to exclude those consumed by moves.
	var filteredRemoves []Operation
	for _, op := range removes {
		if op.Type == -1 {
			continue
		}
		filteredRemoves = append(filteredRemoves, op)
	}
	removes = filteredRemoves

	// 6. Content availability check.
	var missingIDs []c4.ID
	for id := range needsContent {
		found := false
		// Check registered sources.
		for _, src := range r.sources {
			if src.Has(id) {
				found = true
				break
			}
		}
		if !found {
			// Check if content exists elsewhere in the current tree.
			if paths, ok := idToCurrentPaths[id]; ok && len(paths) > 0 {
				found = true
			}
		}
		if !found {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		return &Plan{Missing: missingIDs}, nil
	}

	// 7. Order operations.
	// Mkdirs: shallow first.
	sort.Slice(mkdirs, func(i, j int) bool {
		return depthOf(mkdirs[i].Path) < depthOf(mkdirs[j].Path)
	})
	// Removes: deep first.
	sort.Slice(removes, func(i, j int) bool {
		return depthOf(removes[i].Path) > depthOf(removes[j].Path)
	})
	// Rmdirs: deep first.
	sort.Slice(rmdirs, func(i, j int) bool {
		return depthOf(rmdirs[i].Path) > depthOf(rmdirs[j].Path)
	})

	var ops []Operation
	ops = append(ops, mkdirs...)
	ops = append(ops, moves...)
	ops = append(ops, creates...)
	ops = append(ops, symlinks...)
	ops = append(ops, chmods...)
	ops = append(ops, chtimes...)
	ops = append(ops, removes...)
	ops = append(ops, rmdirs...)

	return &Plan{Operations: ops}, nil
}

// depthOf counts path separators to determine nesting depth.
func depthOf(path string) int {
	return strings.Count(path, string(filepath.Separator))
}
