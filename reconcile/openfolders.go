package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// openFolders gives the owner write and search on every folder inside the
// destination that already exists and that the plan writes into, moves
// out of or removes from (and on each folder above it, which must be
// searched), when its mode lacks them. A folder recorded or held read-only
// (0555, 0500, 0000) otherwise refuses the plan's work inside it. It
// returns each opened folder's own mode, which the post-pass sets back
// (or replaces with the recorded mode), deepest first. Folders the plan
// creates are made owner-writable by applyMkdir instead.
//
// The destination root itself is never opened: it is not in the
// description, and a read-only destination is the user's own statement,
// as it is to cp, rsync and tar. A plan that must write directly into a
// root without owner write and search, or reach inside one without owner
// search, fails here, before anything is opened or written, with an
// error naming the root.
func (r *Reconciler) openFolders(plan *Plan, root string) (map[string]os.FileMode, error) {
	opened := make(map[string]os.FileMode)
	if r.dryRun {
		return opened, nil
	}
	var dirs []string
	for _, op := range plan.Operations {
		switch op.Type {
		case OpMkdir, OpCreate, OpSymlink, OpRemove, OpRmdir:
			dirs = append(dirs, filepath.Dir(op.Path))
		case OpMove:
			dirs = append(dirs, filepath.Dir(op.Path), filepath.Dir(op.SrcPath))
		}
	}

	// The root first, before anything is touched.
	if info, err := os.Lstat(root); err == nil && info.IsDir() {
		perm := info.Mode().Perm()
		for _, dir := range dirs {
			rel, err := filepath.Rel(root, dir)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			if rel == "." && perm&0o300 != 0o300 {
				return nil, fmt.Errorf("%s is not writable (mode %v): the plan writes into it, and a destination's own mode is not changed", root, perm)
			}
			if perm&0o100 == 0 {
				return nil, fmt.Errorf("%s is not searchable (mode %v): the plan works inside it, and a destination's own mode is not changed", root, perm)
			}
		}
	}

	seen := make(map[string]bool)
	for _, dir := range dirs {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		p := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) { // top-down, below the root
			p = filepath.Join(p, part)
			if seen[p] {
				continue
			}
			seen[p] = true
			info, err := os.Lstat(p)
			if err != nil || !info.IsDir() {
				break // absent, or not a folder (a link is never chmodded): nothing below it to open
			}
			perm := info.Mode().Perm()
			if perm&0o300 == 0o300 {
				continue
			}
			if err := os.Chmod(p, perm|0o700); err == nil {
				opened[p] = perm
			}
		}
	}
	return opened, nil
}

// folderPostPass sets folder metadata after everything else, deepest
// first, so setting a folder never refuses or disturbs work inside it: for
// each path, a folder openFolders opened is first set back to its own
// mode, then the folder operations for that path run (meta), which set
// its recorded mode and time. A folder that cannot be set back is named in
// an error: it is left owner-writable, the safe direction.
func folderPostPass(dirOps []Operation, opened map[string]os.FileMode, res *Result, meta func(Operation)) {
	byPath := make(map[string][]Operation)
	var paths []string
	for _, op := range dirOps {
		if _, ok := byPath[op.Path]; !ok {
			paths = append(paths, op.Path)
		}
		byPath[op.Path] = append(byPath[op.Path], op)
	}
	for p := range opened {
		if _, ok := byPath[p]; !ok {
			paths = append(paths, p)
			byPath[p] = nil
		}
	}
	sort.SliceStable(paths, func(i, j int) bool {
		di := strings.Count(paths[i], string(filepath.Separator))
		dj := strings.Count(paths[j], string(filepath.Separator))
		if di != dj {
			return di > dj // deepest first
		}
		return paths[i] > paths[j]
	})
	for _, p := range paths {
		if mode, ok := opened[p]; ok {
			if err := os.Chmod(p, mode); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("folder %s left owner-writable: its mode %v was not set back: %w", p, mode, err))
			}
		}
		for _, op := range byPath[p] {
			meta(op)
		}
	}
}
