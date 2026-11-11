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

	// A target directory with no children listed under it says nothing
	// about its contents unless its C4 ID is the empty listing's: with any
	// other ID it is collapsed (its listing lives under that ID, as in a
	// one-level record), and with none it is unread. Such a directory is
	// left as it is: nothing under it is walked, so nothing under it is
	// planned for removal. Through 1.0.25 every file under it was removed,
	// so `c4 patch` of a one-level record emptied each subdirectory it
	// showed as one line, with exit 0.
	opaque := opaqueDirs(targetPaths)

	// Names the destination folds onto a sibling's (measured there) are
	// refused before planning: the last in byte order is written whole.
	refused, refusedSkip, mayCollide, unmeasured, respell := foldCollisions(dirPath, targetPaths, !r.dryRun)

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
			if info.IsDir() && opaque[rel] {
				return filepath.SkipDir
			}

			// Compute C4 ID for regular files.
			if info.Mode().IsRegular() && info.Size() >= 0 {
				if id, ok := r.trustedID(targetPaths[rel], info); ok {
					currentIDs[rel] = id
					idToCurrentPaths[id] = append(idToCurrentPaths[id], rel)
					return nil
				}
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

	// A target path missing under its exact spelling may still be on disk:
	// a folder that folds names resolves it to an entry listed under
	// another spelling (Readme.md for readme.md). That entry is the
	// target's, by identity, measured with a read-only lookup (so a dry run
	// sees it too): it is never removed, and after Apply it is respelled to
	// the target's exact name. Through 1.0.25 it counted as unlisted and
	// was removed after the target's bytes were written into it, so a
	// description that only changed a file's case left the folder empty.
	aliasOf := make(map[string]string) // current path -> the target path it is
	for relPath := range targetPaths {
		if refusedSkip[relPath] {
			continue
		}
		if _, exact := currentFiles[relPath]; exact {
			continue
		}
		info, err := os.Lstat(filepath.Join(dirPath, filepath.FromSlash(strings.TrimSuffix(relPath, "/"))))
		if err != nil {
			continue
		}
		for cur, ci := range currentFiles {
			if _, taken := aliasOf[cur]; taken || targetPaths[cur] != nil {
				continue
			}
			if os.SameFile(ci, info) {
				aliasOf[cur] = relPath
				respell = append(respell, relPath)
				break
			}
		}
	}
	// Respell top-down, so a directory has its exact name before its
	// children are respelled inside it.
	sort.Slice(respell, func(i, j int) bool {
		di, dj := depthOf(respell[i]), depthOf(respell[j])
		if di != dj {
			return di < dj
		}
		return respell[i] < respell[j]
	})

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
	// Folded sequence entries name an ID-list object, not file bytes, and
	// this reconciler cannot expand them into member files. Writing the
	// ID list under the range pattern's name (and removing the real
	// members it does not account for) would be silent corruption, so a
	// folded entry is reported as missing and the plan does nothing.
	var unexpandable []c4.ID
	// Track which current paths are accounted for by the target.
	targetAccountedFor := make(map[string]bool)

	// Process target entries.
	for relPath, entry := range targetPaths {
		if refusedSkip[relPath] {
			targetAccountedFor[relPath] = true
			continue
		}
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

		if entry.IsSequence {
			unexpandable = append(unexpandable, entry.C4ID)
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
	//
	// A name the target records must never be removed, even when its TYPE
	// differs from what is on disk. Directory paths carry a trailing slash
	// and non-directories do not, so a name that is a directory in the
	// target and a symlink in the destination lands under two different
	// keys — and was planned as both a create and a remove. Restore then
	// could not converge: the remove failed against the directory it had
	// just correctly built, and reported the whole restore incomplete.
	accounted := func(relPath string) bool {
		base := strings.TrimSuffix(relPath, "/")
		return targetAccountedFor[base] || targetAccountedFor[base+"/"]
	}
	for relPath, info := range currentFiles {
		if accounted(relPath) || aliasOf[relPath] != "" {
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
	missingIDs := unexpandable
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
		return &Plan{Missing: missingIDs, Refused: refused, MayCollide: mayCollide, Unmeasured: unmeasured, Respell: respell}, nil
	}

	// 7. Restore parent directory times. Every create, move, symlink,
	//    remove and mkdir rewrites its parent directory, bumping its mtime
	//    — including directories whose time already matched the target
	//    and so drew no OpChtimes above. Schedule one for each such
	//    parent so Apply's directory post-pass resets it after all writes.
	scheduled := make(map[string]bool, len(chtimes)+len(mkdirs))
	for _, op := range chtimes {
		scheduled[op.Path] = true
	}
	for _, op := range mkdirs {
		scheduled[op.Path] = true
	}
	touch := func(path string) {
		parent := filepath.Dir(path)
		if scheduled[parent] {
			return
		}
		scheduled[parent] = true
		rel, err := filepath.Rel(dirPath, parent)
		if err != nil || rel == "." {
			return
		}
		entry := targetPaths[filepath.ToSlash(rel)+"/"]
		if entry == nil || !entry.IsDir() || entry.Timestamp.Equal(c4m.NullTimestamp()) {
			return
		}
		chtimes = append(chtimes, Operation{Type: OpChtimes, Path: parent, Entry: entry})
	}
	for _, ops := range [][]Operation{mkdirs, creates, symlinks, removes, rmdirs} {
		for _, op := range ops {
			touch(op.Path)
		}
	}
	for _, op := range moves {
		touch(op.Path)
		touch(op.SrcPath)
	}

	// 8. Order operations.
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

	return &Plan{Operations: ops, Refused: refused, MayCollide: mayCollide, Unmeasured: unmeasured, Respell: respell}, nil
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

// emptyListingID is the C4 ID of an empty listing: the ID of zero bytes.
var emptyListingID = c4.Identify(strings.NewReader(""))

// opaqueDirs returns the target directories whose contents the target does
// not state: no path is listed under them and their C4 ID is not the empty
// listing's (collapsed, with another ID, or unread, with none).
func opaqueDirs(targetPaths map[string]*c4m.Entry) map[string]bool {
	hasChild := make(map[string]bool)
	for p := range targetPaths {
		parent := strings.TrimSuffix(p, "/")
		if i := strings.LastIndex(parent, "/"); i >= 0 {
			hasChild[parent[:i+1]] = true
		}
	}
	out := make(map[string]bool)
	for p, e := range targetPaths {
		if strings.HasSuffix(p, "/") && !hasChild[p] && e.C4ID != emptyListingID {
			out[p] = true
		}
	}
	return out
}

// depthOf counts path separators to determine nesting depth.
func depthOf(path string) int {
	return strings.Count(path, string(filepath.Separator))
}
