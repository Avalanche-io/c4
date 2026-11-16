package store

import (
	"os"
	"path/filepath"

	"github.com/Avalanche-io/c4"
)

// Misplaced is an object file found above the level a lookup resolves its
// ID to: a write that resolved its target before another process's split
// left it there, or a split cut short by a crash did.
type Misplaced struct {
	ID   c4.ID
	Path string // where the file is
	Want string // where the trie puts it now
	// Moved is true when Settle moved the file to Want. A file whose Want
	// already holds the object is left where it is: the ID names the same
	// bytes, and nothing is removed.
	Moved bool
}

// Settle walks the whole store and reports every object file above its
// proper level, moving each to its proper place when move is true. A store
// written before 2026-11-16 by processes sharing it can hold such files
// (the store split race). This version's lookups find them where they are
// (foldFind), so a store needs no repair to read correctly here; moving
// them is for readers built before the fix, which still miss them. Settle
// lists every directory, a cost of the order of one Walk; it holds the
// store's lock for each move, and a file only ever moves deeper, so it is
// safe beside other processes using the store.
func (s *TreeStore) Settle(move bool) ([]Misplaced, error) {
	var out []Misplaced
	err := filepath.WalkDir(s.root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // moved or removed by another process meanwhile
			}
			return err
		}
		if d.IsDir() || isTemp(d.Name()) {
			return nil
		}
		id, perr := c4.Parse(d.Name())
		if perr != nil {
			return nil
		}
		want := s.path(id)
		if want == p || !deeper(want, p) {
			return nil
		}
		m := Misplaced{ID: id, Path: p, Want: want}
		if move {
			s.mu.Lock()
			if _, err := s.statPath(want); os.IsNotExist(err) {
				if err := os.Rename(p, want); err == nil {
					m.Moved = true
				}
			}
			s.mu.Unlock()
		}
		out = append(out, m)
		return nil
	})
	return out, err
}

// deeper reports whether want lies in a directory strictly below the
// directory holding p, so that moving p to want moves it down the trie,
// the only direction a file ever moves.
func deeper(want, p string) bool {
	rel, err := filepath.Rel(filepath.Dir(p), filepath.Dir(want))
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !filepath.IsAbs(rel) && !startsWithDotDot(rel)
}

func startsWithDotDot(rel string) bool {
	return len(rel) >= 2 && rel[:2] == ".." && (len(rel) == 2 || os.IsPathSeparator(rel[2]))
}
