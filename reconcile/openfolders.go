package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// openFolders gives the owner write and search on every folder that
// already exists in the destination and that the plan writes into, moves
// out of or removes from (and on each folder above it, which must be
// searched), when its mode lacks them. A folder recorded or held read-only
// (0555, 0500, 0000) otherwise refuses the plan's work inside it. It
// returns each opened folder's own mode, which the post-pass sets back
// (or replaces with the recorded mode), deepest first. Folders the plan
// creates are made owner-writable by applyMkdir instead.
func (r *Reconciler) openFolders(plan *Plan, root string) map[string]os.FileMode {
	opened := make(map[string]os.FileMode)
	if r.dryRun {
		return opened
	}
	seen := make(map[string]bool)
	open := func(dir string) {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return
		}
		chain := []string{root}
		if rel != "." {
			p := root
			for _, part := range strings.Split(rel, string(filepath.Separator)) {
				p = filepath.Join(p, part)
				chain = append(chain, p)
			}
		}
		for _, p := range chain { // top-down: a folder is opened before its child is examined
			if seen[p] {
				continue
			}
			seen[p] = true
			info, err := os.Lstat(p)
			if err != nil || !info.IsDir() {
				return // absent, or not a folder: nothing below it exists to open
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
	for _, op := range plan.Operations {
		switch op.Type {
		case OpMkdir, OpCreate, OpSymlink, OpRemove, OpRmdir:
			open(filepath.Dir(op.Path))
		case OpMove:
			open(filepath.Dir(op.Path))
			open(filepath.Dir(op.SrcPath))
		}
	}
	return opened
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
