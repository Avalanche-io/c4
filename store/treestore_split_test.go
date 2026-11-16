package store

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4"
)

// Two TreeStore values on one directory stand for two processes: the
// store's mutex is per value, as it is per process. A split moves a leaf's
// files one level down, one rename at a time, after making the
// subdirectories; through 2026-11-16 a lookup descended into any
// subdirectory that existed and looked nowhere else, so (a) from the mkdir
// until its own rename a file was invisible to every other process, and
// (b) a writer that resolved its target before another process's split
// renamed into the old level, where no lookup looked again: Put returned
// an ID that no store could find.

func splitTestStore(t *testing.T, dir string, threshold int) *TreeStore {
	t.Helper()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSplitThreshold(threshold)
	s.SetSyncMode(SyncNone)
	return s
}

func putRandom(t testing.TB, s *TreeStore) c4.ID {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	id, err := s.Put(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// (a) Objects present throughout stay visible to another process while a
// split moves them. Measured on master b82ec07 at the default threshold:
// in every run thousands of 12,288 Has calls returned false.
func TestTreeStoreSplitKeepsObjectsVisible(t *testing.T) {
	dir := t.TempDir()
	const threshold = DefaultSplitThreshold
	w := splitTestStore(t, dir, threshold)
	ids := make([]c4.ID, 0, threshold)
	for i := 0; i < threshold; i++ { // at the threshold: the next Put splits
		ids = append(ids, putRandom(t, w))
	}
	o := splitTestStore(t, dir, threshold) // the other process
	for _, id := range ids {
		if !o.Has(id) {
			t.Fatal("precondition: an object is missing before the split")
		}
	}

	var stop int32
	var calls, misses, openMisses int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for pass := 0; atomic.LoadInt32(&stop) == 0 || pass < 1; pass++ {
			for i, id := range ids {
				calls++
				if !o.Has(id) {
					misses++
				}
				if i%8 == 0 {
					f, err := o.Open(id)
					if err != nil {
						openMisses++
						continue
					}
					f.Close()
				}
			}
		}
	}()
	putRandom(t, w) // crosses the threshold: the root leaf splits
	atomic.StoreInt32(&stop, 1)
	wg.Wait()
	if _, err := os.Stat(filepath.Join(dir, "c4")); err != nil {
		t.Fatalf("precondition: no split happened: %v", err)
	}
	if misses != 0 || openMisses != 0 {
		t.Errorf("during one split, %d of %d Has calls on objects present throughout returned false, and %d Open calls failed",
			misses, calls, openMisses)
	}
}

// (b) Every ID Put returned is found by a fresh store after several
// processes wrote across a split at once. Measured on master b82ec07 at
// threshold 16: about 30% of rounds left objects one level too high.
func TestTreeStoreConcurrentPublishAcrossSplit(t *testing.T) {
	const rounds, writers, each, threshold = 60, 8, 12, 16
	lostRounds, lost := 0, 0
	for r := 0; r < rounds; r++ {
		dir := t.TempDir()
		pre := splitTestStore(t, dir, threshold)
		for i := 0; i < threshold-4; i++ { // near the threshold
			putRandom(t, pre)
		}
		var mu sync.Mutex
		var ids []c4.ID
		var wg sync.WaitGroup
		for k := 0; k < writers; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s, err := NewTreeStore(dir) // each writer its own process
				if err != nil {
					t.Error(err)
					return
				}
				s.SetSplitThreshold(threshold)
				s.SetSyncMode(SyncNone)
				for i := 0; i < each; i++ {
					b := make([]byte, 24)
					rand.Read(b)
					id, err := s.Put(bytes.NewReader(b))
					if err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					ids = append(ids, id)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		fresh := splitTestStore(t, dir, threshold)
		n := 0
		for _, id := range ids {
			if !fresh.Has(id) {
				n++
				continue
			}
			f, err := fresh.Open(id)
			if err != nil {
				n++
				continue
			}
			f.Close()
		}
		if n > 0 {
			lostRounds++
			lost += n
		}
	}
	if lost != 0 {
		t.Errorf("%d of %d rounds (%d writers x %d puts, threshold %d) left %d objects that Put returned and a fresh store cannot find",
			lostRounds, rounds, writers, each, threshold, lost)
	}
}

// An object one level above its proper place, as a racing publish or a
// split cut short by a crash leaves it, is found by every lookup, so a
// store damaged before the fix reads correctly with no repair.
func TestTreeStoreFindsObjectAboveItsLevel(t *testing.T) {
	root := t.TempDir()
	s := splitTestStore(t, root, 4)
	var ids []c4.ID
	for i := 0; i < 40; i++ { // several splits: c4/ and some of c4/XX/
		ids = append(ids, putRandom(t, s))
	}
	for _, id := range ids {
		p, ok := s.ContentPath(id)
		if !ok {
			t.Fatalf("precondition: %s missing", id)
		}
		if filepath.Dir(p) == root {
			continue
		}
		// Move it up one level, as a publish that resolved its path
		// before another process's split leaves it.
		up := filepath.Join(filepath.Dir(filepath.Dir(p)), id.String())
		if err := os.Rename(p, up); err != nil {
			t.Fatal(err)
		}
		r := splitTestStore(t, root, 4)
		if !r.Has(id) {
			t.Errorf("Has: an object one level up (%s) is not found", relTo(root, up))
		}
		if cp, ok := r.ContentPath(id); !ok || cp != up {
			t.Errorf("ContentPath = %q, %v; want %q", cp, ok, up)
		}
		f, err := r.Open(id)
		if err != nil {
			t.Errorf("Open: %v", err)
		} else {
			f.Close()
		}
		// A Put of the same content finds it and does not write a copy.
		if _, ok := r.locate(id); !ok {
			t.Errorf("locate: not found")
		}
		if err := r.Remove(id); err != nil {
			t.Errorf("Remove: %v", err)
		}
		if _, err := os.Stat(up); !os.IsNotExist(err) {
			t.Errorf("Remove left the object at %s", relTo(root, up))
		}
		return
	}
	t.Fatal("fixture: no object below the root")
}

func relTo(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}

// Settle reports every object above its proper level, moves them with
// move set, and leaves a store a reader built before the fix (exact trie
// path only) reads in full. A copy whose proper place already holds the
// object is reported and left alone.
func TestTreeStoreSettle(t *testing.T) {
	root := t.TempDir()
	s := splitTestStore(t, root, 4)
	var ids []c4.ID
	for i := 0; i < 60; i++ {
		ids = append(ids, putRandom(t, s))
	}
	oldReader := func(id c4.ID) bool { // what lookups did through 2026-11-16
		_, err := os.Stat(s.path(id))
		return err == nil
	}
	var up []c4.ID
	var dup c4.ID
	for _, id := range ids {
		p, _ := s.ContentPath(id)
		if filepath.Dir(p) == root {
			continue
		}
		dst := filepath.Join(filepath.Dir(filepath.Dir(p)), id.String())
		if dup.IsNil() {
			// A duplicate: the object at its place and a copy one level up.
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				t.Fatal(err)
			}
			dup = id
			continue
		}
		if err := os.Rename(p, dst); err != nil {
			t.Fatal(err)
		}
		up = append(up, id)
		if len(up) == 5 {
			break
		}
	}
	if len(up) != 5 || dup.IsNil() {
		t.Fatalf("fixture: %d objects moved up", len(up))
	}
	for _, id := range up {
		if oldReader(id) {
			t.Fatal("fixture: an exact-path lookup still finds a moved-up object")
		}
	}

	report, err := s.Settle(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 6 {
		t.Errorf("Settle(false) reports %d misplaced files, want 6 (5 moved up and 1 duplicate)", len(report))
	}
	for _, m := range report {
		if m.Moved {
			t.Errorf("Settle(false) moved %s", m.ID)
		}
	}
	for _, id := range up {
		if oldReader(id) {
			t.Error("Settle(false) moved a file")
		}
	}

	report, err = s.Settle(true)
	if err != nil {
		t.Fatal(err)
	}
	moved := 0
	for _, m := range report {
		if m.Moved {
			moved++
		} else if m.ID != dup {
			t.Errorf("Settle(true) did not move %s", m.ID)
		}
	}
	if moved != 5 {
		t.Errorf("Settle(true) moved %d files, want 5", moved)
	}
	for _, id := range ids {
		if !oldReader(id) {
			t.Errorf("after Settle(true), an exact-path lookup misses %s", id)
		}
	}
	report, err = s.Settle(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 || report[0].ID != dup {
		t.Errorf("after Settle(true), Settle(false) reports %d files, want only the duplicate", len(report))
	}
	// Remove deletes both copies of the duplicate, so Has stops finding it.
	if err := s.Remove(dup); err != nil {
		t.Fatal(err)
	}
	if s.Has(dup) {
		t.Errorf("Remove left a copy of %s that Has still finds", dup.String()[:12])
	}
}

// A lookup looks at each level before the levels below it, the order files
// move in. Here the other process's split moves the file down at the worst
// moment for a lookup that looked below first: just after the lookup's
// search has looked for it one level down, and before it looks at the
// level the file was on. Looking top down, the lookup has already found it.
func TestTreeStoreLookupOrderBeatsASplitMove(t *testing.T) {
	root := t.TempDir()
	s := splitTestStore(t, root, DefaultSplitThreshold)
	id := putRandom(t, s)
	str := id.String()
	shallow := filepath.Join(root, str)
	sub := filepath.Join(root, str[0:2])
	deep := filepath.Join(sub, str)
	// Mid-split: the subdirectory exists and the file has not moved yet.
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	deepLooks := 0
	r := splitTestStore(t, root, DefaultSplitThreshold)
	r.stat = func(p string) (os.FileInfo, error) {
		info, err := os.Stat(p)
		if p == deep && os.IsNotExist(err) {
			deepLooks++
			if deepLooks == 2 { // the search's own look below, not the fast path's
				if rerr := os.Rename(shallow, deep); rerr != nil {
					t.Error(rerr)
				}
			}
		}
		return info, err
	}
	if !r.Has(id) {
		t.Errorf("Has: the file moved down while the lookup was between levels, and was missed")
	}
}

// A split moves only objects. The store root holds the journal, and
// through 2026-11-16 the root's split moved it to jo/journal, where the
// journal no longer read it (measured at the default threshold with real
// `c4 id -s` runs: nine claims became one). Any other file kept in a leaf
// stays too, and so do temp files.
func TestTreeStoreSplitMovesOnlyObjects(t *testing.T) {
	root := t.TempDir()
	keep := []string{"journal", "notes.txt", ".ingest.host.1.x", "c4-not-an-id"}
	for _, name := range keep {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := splitTestStore(t, root, 4)
	var ids []c4.ID
	for i := 0; i < 12; i++ {
		ids = append(ids, putRandom(t, s))
	}
	if _, err := os.Stat(filepath.Join(root, "c4")); err != nil {
		t.Fatalf("precondition: the root did not split: %v", err)
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("the root's split moved %s: %v", name, err)
		}
	}
	for _, d := range []string{"jo", "no", "c4-"} {
		if _, err := os.Stat(filepath.Join(root, d)); err == nil {
			t.Errorf("the split made a directory %s/ for a file that is not an object", d)
		}
	}
	for _, id := range ids {
		p, ok := s.ContentPath(id)
		if !ok || filepath.Dir(p) == root {
			t.Errorf("object %s not moved below the root: %q %v", id.String()[:12], p, ok)
		}
	}
}

// The premises the top-down lookup rests on (foldFind), pinned over real
// splits: an object only ever moves deeper, and no shard directory is ever
// removed. Every directory that existed before a Put still exists after
// it, and every object is where it was or below it.
func TestTreeStoreSplitLeavesItsLevel(t *testing.T) {
	root := t.TempDir()
	s := splitTestStore(t, root, 4)
	where := map[c4.ID]string{}
	dirs := map[string]bool{}
	snapshot := func() map[string]bool {
		out := map[string]bool{}
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				out[p] = true
			}
			return nil
		})
		return out
	}
	for i := 0; i < 400; i++ {
		where[putRandom(t, s)] = ""
		now := snapshot()
		for d := range dirs {
			if !now[d] {
				t.Fatalf("Put %d removed the shard directory %s", i, relTo(root, d))
			}
		}
		dirs = now
		for id, before := range where {
			p, ok := s.ContentPath(id)
			if !ok {
				t.Fatalf("Put %d: %s is missing", i, id.String()[:12])
			}
			if before != "" && p != before && !deeper(p, before) {
				t.Fatalf("Put %d moved %s up or sideways: %s to %s", i, id.String()[:12], relTo(root, before), relTo(root, p))
			}
			where[id] = p
		}
	}
	if len(dirs) < 3 {
		t.Fatalf("fixture: only %d directories; the splits did not go two levels deep", len(dirs))
	}
}

// After the fix a writer can still publish at a stale level: Put resolves
// its target under its own process's lock, and another process's split can
// come between that and the rename. What it leaves is an object one level
// up, which every lookup finds, or a second copy of an ID already stored,
// with the same bytes. Here several processes Put the same contents and
// their own across splits; then every ID reads back with bytes that hash
// to it, Settle loses nothing, and Remove leaves no copy behind.
func TestTreeStoreStaleLevelWriters(t *testing.T) {
	const rounds, writers, threshold = 40, 8, 16
	shared := make([][]byte, 12)
	for i := range shared {
		shared[i] = []byte(fmt.Sprintf("shared content %d", i))
	}
	dupRounds, upRounds := 0, 0
	for r := 0; r < rounds; r++ {
		dir := t.TempDir()
		pre := splitTestStore(t, dir, threshold)
		for i := 0; i < threshold-4; i++ {
			putRandom(t, pre)
		}
		var mu sync.Mutex
		ids := map[c4.ID][]byte{}
		var wg sync.WaitGroup
		for k := 0; k < writers; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				s, err := NewTreeStore(dir)
				if err != nil {
					t.Error(err)
					return
				}
				s.SetSplitThreshold(threshold)
				s.SetSyncMode(SyncNone)
				for i := 0; i < len(shared); i++ {
					for _, b := range [][]byte{shared[(i+k)%len(shared)], []byte(fmt.Sprintf("writer %d put %d round %d", k, i, r))} {
						id, err := s.Put(bytes.NewReader(b))
						if err != nil {
							t.Error(err)
							return
						}
						mu.Lock()
						ids[id] = b
						mu.Unlock()
					}
				}
			}(k)
		}
		wg.Wait()

		// What the round left on disk: copies per ID, and objects above
		// their level.
		fresh := splitTestStore(t, dir, threshold)
		copies := map[c4.ID]int{}
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if id, perr := c4.Parse(d.Name()); perr == nil {
					copies[id]++
				}
			}
			return nil
		})
		for _, n := range copies {
			if n > 1 {
				dupRounds++
				break
			}
		}
		if m, _ := fresh.Settle(false); len(m) > 0 {
			upRounds++
		}

		readsBack := func(when string) {
			for id, want := range ids {
				f, err := fresh.Open(id)
				if err != nil {
					t.Fatalf("round %d, %s: %s: %v", r, when, id.String()[:12], err)
				}
				got, err := io.ReadAll(f)
				f.Close()
				if err != nil || !bytes.Equal(got, want) || c4.Identify(bytes.NewReader(got)) != id {
					t.Fatalf("round %d, %s: %s reads back wrong bytes", r, when, id.String()[:12])
				}
			}
		}
		readsBack("after the writers")
		if _, err := fresh.Settle(true); err != nil {
			t.Fatal(err)
		}
		readsBack("after Settle")
		for id := range ids {
			if err := fresh.Remove(id); err != nil {
				t.Fatalf("round %d: Remove %s: %v", r, id.String()[:12], err)
			}
			if fresh.Has(id) {
				t.Fatalf("round %d: Remove %s left a copy that Has still finds", r, id.String()[:12])
			}
		}
	}
	t.Logf("%d of %d rounds left a second copy of some ID, %d left an object above its level; nothing lost or wrong", dupRounds, rounds, upRounds)
}
