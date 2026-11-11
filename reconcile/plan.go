package reconcile

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

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
	currentIDs := make(map[string]c4.ID)         // relative path -> C4 ID
	idToCurrentPaths := make(map[c4.ID][]string) // C4 ID -> relative paths

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
				if id, ok := r.knownID(rel, info); ok {
					currentIDs[rel] = id
					idToCurrentPaths[id] = append(idToCurrentPaths[id], rel)
					return nil
				}
				if id, ok := r.trustedID(targetPaths[rel], info); ok {
					currentIDs[rel] = id
					idToCurrentPaths[id] = append(idToCurrentPaths[id], rel)
					return nil
				}
				f, ferr := os.Open(path)
				if ferr == nil {
					// A file whose read fails has no ID: it is a present
					// path with unknown content, never a file holding
					// the bytes read before the failure.
					id, herr := c4.IdentifyReader(f)
					f.Close()
					if herr != nil {
						return nil
					}
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
			curInfo, exists := currentFiles[relPath]
			if !exists {
				mkdirs = append(mkdirs, Operation{
					Type:  OpMkdir,
					Path:  absPath,
					Entry: entry,
				})
			} else {
				// Directory exists — check if metadata needs updating.
				if !entry.Timestamp.Equal(c4m.NullTimestamp()) &&
					!curInfo.ModTime().UTC().Truncate(time.Second).Equal(entry.Timestamp.UTC().Truncate(time.Second)) {
					chtimes = append(chtimes, Operation{
						Type:  OpChtimes,
						Path:  absPath,
						Entry: entry,
					})
				}
				if runtime.GOOS != "windows" &&
					entry.Mode != 0 && curInfo.Mode().Perm() != entry.Mode.Perm() {
					chmods = append(chmods, Operation{
						Type:  OpChmod,
						Path:  absPath,
						Entry: entry,
					})
				}
			}
			continue
		}

		if entry.IsSymlink() {
			curInfo, exists := currentFiles[relPath]
			if exists && curInfo.Mode()&os.ModeSymlink != 0 {
				// Check if symlink target matches.
				curTarget, lerr := os.Readlink(filepath.Join(dirPath, filepath.FromSlash(relPath)))
				if lerr == nil && curTarget == entry.Target {
					// The target is right; the link's own time may not be.
					// A null time never overwrites (null rules), so only a
					// stated time that differs is set.
					if !entry.Timestamp.Equal(c4m.NullTimestamp()) &&
						!curInfo.ModTime().Truncate(time.Second).Equal(entry.Timestamp.Truncate(time.Second)) {
						chtimes = append(chtimes, Operation{
							Type:  OpChtimes,
							Path:  absPath,
							Entry: entry,
						})
					}
					continue
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
					!curInfo.ModTime().UTC().Truncate(time.Second).Equal(entry.Timestamp.UTC().Truncate(time.Second))

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

	// A removed file moves to the first target that wants its content; a
	// second target for the same content (one plate in two shot folders)
	// is a create copied from where the move put it. Moves run before
	// creates, so that path exists by then. Before this, the remove stayed
	// in removeByID after the move consumed it, so every later target
	// became a second move from the same source, and the rename failed
	// with the folder half switched (found by a demo scout, 2026-11-03).
	movedTo := make(map[c4.ID]string) // C4 ID -> absolute path a move put it at
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
			// Mark the remove as consumed, once.
			removes[rmIdx].Type = -1 // sentinel
			delete(removeByID, op.ContentID)
			movedTo[op.ContentID] = op.Path
			continue
		}
		if dst, ok := movedTo[op.ContentID]; ok {
			op.SrcPath = dst
		}
		filteredCreates = append(filteredCreates, op)
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

	// 5b. A directory whose contents this plan changes gets its recorded
	//     mtime set back afterwards, whether or not its own mtime differs
	//     now: creating (temp file plus rename), moving, linking or
	//     removing an entry bumps the parent's mtime on every platform, so
	//     a directory that matched the target at plan time no longer
	//     matches after Apply, and a re-run then plans the chtimes it
	//     should have planned the first time. Apply runs directory
	//     chtimes in a post-pass, deepest first, after the children.
	//     (2026-11-03: a changed file inside a subdirectory failed
	//     restore's verification on every run.)
	hasChtimes := make(map[string]bool, len(chtimes))
	for _, op := range chtimes {
		hasChtimes[op.Path] = true
	}
	touched := make(map[string]bool)
	noteParents := func(p string) {
		for d := filepath.Dir(p); len(d) > len(dirPath); d = filepath.Dir(d) {
			touched[d] = true
		}
	}
	for _, op := range creates {
		noteParents(op.Path)
	}
	for _, op := range moves {
		noteParents(op.Path)
		noteParents(op.SrcPath)
	}
	for _, op := range symlinks {
		noteParents(op.Path)
	}
	for _, op := range removes {
		noteParents(op.Path)
	}
	for _, op := range mkdirs {
		noteParents(op.Path)
	}
	for _, op := range rmdirs {
		noteParents(op.Path)
	}
	touchedDirs := make([]string, 0, len(touched))
	for d := range touched {
		touchedDirs = append(touchedDirs, d)
	}
	sort.Strings(touchedDirs) // byte-identical plans at any concurrency
	for _, abs := range touchedDirs {
		if hasChtimes[abs] {
			continue
		}
		rel, err := filepath.Rel(dirPath, abs)
		if err != nil {
			continue
		}
		entry := targetPaths[filepath.ToSlash(rel)+"/"]
		if entry == nil || entry.Timestamp.Equal(c4m.NullTimestamp()) {
			continue // not described, or no time recorded: the platform's time stands
		}
		chtimes = append(chtimes, Operation{
			Type:  OpChtimes,
			Path:  abs,
			Entry: entry,
		})
	}

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

// trustedID returns the target entry's C4 ID without hashing when
// trusted-metadata planning is enabled and the file's size and mtime
// (second precision) match the entry — the guided-scan contract.
func (r *Reconciler) trustedID(e *c4m.Entry, info os.FileInfo) (c4.ID, bool) {
	if !r.trustMetadata || e == nil || e.C4ID.IsNil() || e.IsDir() || e.IsSymlink() {
		return c4.ID{}, false
	}
	if e.Size != info.Size() || e.Timestamp.Equal(c4m.NullTimestamp()) {
		return c4.ID{}, false
	}
	if !info.ModTime().UTC().Truncate(time.Second).Equal(e.Timestamp.UTC().Truncate(time.Second)) {
		return c4.ID{}, false
	}
	return e.C4ID, true
}

// knownID returns the C4 ID the caller's own scan (WithKnownState)
// recorded for a file, when the file is provably the one that scan
// hashed: same size, same mtime, and an mtime strictly older than the
// scan's start at second precision, so a write during or after the scan
// (within the filesystem's timestamp granularity) is always rehashed.
func (r *Reconciler) knownID(rel string, info os.FileInfo) (c4.ID, bool) {
	e := r.known[rel]
	if e == nil || e.C4ID.IsNil() || e.IsDir() || e.IsSymlink() || e.Size != info.Size() {
		return c4.ID{}, false
	}
	if e.Timestamp.Equal(c4m.NullTimestamp()) {
		return c4.ID{}, false
	}
	mtime := info.ModTime().UTC().Truncate(time.Second)
	if !mtime.Equal(e.Timestamp.UTC().Truncate(time.Second)) || !mtime.Before(r.knownStart.Truncate(time.Second)) {
		return c4.ID{}, false
	}
	return e.C4ID, true
}

// depthOf counts path separators to determine nesting depth.
func depthOf(path string) int {
	return strings.Count(path, string(filepath.Separator))
}
