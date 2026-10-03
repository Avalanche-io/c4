package scan

import (
	"os"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/internal/racy"
)

// noteRacy collects a regular file whose mtime second is at or after the
// scan's start: it could still be rewritten, with the same size, inside
// that second after the walk read it. Structure mode records no
// timestamps and so has nothing to protect.
func (g *Generator) noteRacy(e *Entry, path string, info os.FileInfo) {
	if g.mode == ModeStructure || !info.Mode().IsRegular() || !racy.Is(info.ModTime(), g.scanStart) {
		return
	}
	g.racyMu.Lock()
	g.racyFiles = append(g.racyFiles, racyFile{entry: e, path: path})
	g.racyMu.Unlock()
}

// settleRacy waits out and re-observes the racy files collected by the
// walk (see internal/racy), records a null timestamp for any still
// changing after the bounded rounds, and re-resolves the directories
// above them. entries must be in walk order.
func (g *Generator) settleRacy(entries []*Entry) {
	files := g.racyFiles
	g.racyFiles = nil
	if len(files) == 0 {
		return
	}
	unsettled := racy.Settle(len(files),
		func(i int) time.Time { return files[i].entry.Timestamp },
		func(i int) (time.Time, bool) { return g.reobserve(files[i]) })
	for _, i := range unsettled {
		files[i].entry.Timestamp = c4m.NullTimestamp()
	}

	// Directory sizes and IDs are resolved during the walk only in
	// ModeFull; the other modes resolve sizes in the final whole-manifest
	// pass, which has not run yet.
	if g.mode != ModeFull {
		return
	}
	changed := make(map[*Entry]bool, len(files))
	for _, f := range files {
		changed[f.entry] = true
	}
	g.refreshDirs(entries, changed)
}

// reobserve re-reads one racy file — stat, hash, stat — and records the
// result in its entry. It returns the new mtime and whether the two stats
// agree. A file that is gone, or is no longer a regular file, can no
// longer be observed: its timestamp is recorded null and it is settled.
func (g *Generator) reobserve(f racyFile) (time.Time, bool) {
	stat := os.Lstat
	if g.followSymlinks {
		stat = os.Stat
	}
	e := f.entry
	before, err := stat(f.path)
	if err != nil || !before.Mode().IsRegular() {
		e.Timestamp = c4m.NullTimestamp()
		return time.Time{}, true
	}
	var id c4.ID
	if g.mode == ModeFull {
		if id, err = g.computeFileC4ID(f.path); err != nil {
			e.Timestamp = c4m.NullTimestamp()
			return time.Time{}, true
		}
	}
	after, err := stat(f.path)
	if err != nil || !after.Mode().IsRegular() {
		e.Timestamp = c4m.NullTimestamp()
		return time.Time{}, true
	}
	e.Mode = after.Mode()
	e.Size = after.Size()
	e.Timestamp = after.ModTime().UTC()
	if g.mode == ModeFull {
		e.C4ID = id
	}
	return after.ModTime(), after.Size() == before.Size() && after.ModTime().Equal(before.ModTime())
}

// refreshDirs re-resolves, bottom-up, every directory above a changed
// entry, using the same per-directory step as the walk (resolveDir).
// entries must be in walk order: each directory's self-entry followed
// contiguously by its descendants.
func (g *Generator) refreshDirs(entries []*Entry, changed map[*Entry]bool) {
	type frame struct {
		dir      *Entry
		children []*Entry
		dirty    bool
	}
	var stack []*frame
	pop := func() {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !f.dirty {
			return
		}
		f.dir.Size = -1 // resolved from the children again
		g.resolveDir(f.dir, f.children)
		if len(stack) > 0 {
			stack[len(stack)-1].dirty = true
		}
	}
	for _, e := range entries {
		for len(stack) > 0 && stack[len(stack)-1].dir.Depth >= e.Depth {
			pop()
		}
		if len(stack) > 0 {
			top := stack[len(stack)-1]
			top.children = append(top.children, e)
			if changed[e] {
				top.dirty = true
			}
		}
		if e.IsDir() {
			stack = append(stack, &frame{dir: e})
		}
	}
	for len(stack) > 0 {
		pop()
	}
}
