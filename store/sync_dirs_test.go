package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// syncRecorder replaces the barrier's two directory syncs and records
// each call with whether every pending object had already been renamed
// to its hash name at that moment (the barrier empties the pending set
// as it renames), so "after the renames" is observable.
type syncRecorder struct {
	s       *TreeStore
	ids     []c4.ID
	full    []syncCall
	entries []syncCall
}

type syncCall struct {
	dir          string
	allPublished bool
}

func (r *syncRecorder) published() bool { return len(r.s.pending) == 0 }

// finals returns the objects' paths as the trie holds them now; shard
// splits during publication move objects, so read after Sync.
func (r *syncRecorder) finals() []string {
	var out []string
	for _, id := range r.ids {
		out = append(out, r.s.path(id))
	}
	return out
}

func (r *syncRecorder) install(t *testing.T) {
	t.Helper()
	oldFull, oldEntries := syncDirFull, syncDirEntries
	syncDirFull = func(d string) error {
		r.full = append(r.full, syncCall{d, r.published()})
		return oldFull(d)
	}
	syncDirEntries = func(d string) error {
		r.entries = append(r.entries, syncCall{d, r.published()})
		return oldEntries(d)
	}
	t.Cleanup(func() { syncDirFull, syncDirEntries = oldFull, oldEntries })
}

func putBatch(t *testing.T, s *TreeStore, from, n int) []c4.ID {
	t.Helper()
	var ids []c4.ID
	for i := from; i < from+n; i++ {
		id, err := s.Put(bytes.NewReader([]byte(fmt.Sprintf("object %d\n", i))))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func within(root, d string) bool {
	rel, err := filepath.Rel(root, d)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// After Sync, every directory that received a name and each of its
// ancestors up to the root has had its entries synced, after the
// renames; the device flush happened before the renames and once again
// after the entry syncs; and nothing outside the root was touched. Two
// batches: the first splits shards (so split sources and targets must be
// covered), the second lands in existing shards without splitting (so
// the ancestors must be covered by the walk, not by the split marking).
//
// Broken variants this test rejects (each tried while writing it): the
// entry-sync loop removed (no entries synced); entry syncs issued before
// the renames (allPublished false); the leaf directory alone without its
// ancestors (second batch: fewer directories than wanted); the final
// device flush removed (one flush recorded, not two).
func TestSyncSyncsEveryTouchedDirectoryAfterTheRenames(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	s, err := NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSyncMode(SyncBatch)
	s.SetSplitThreshold(2) // force shard directories so ancestors matter
	rec := &syncRecorder{s: s}
	rec.install(t)
	rec.ids = putBatch(t, s, 0, 12)
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	checkBarrier(t, rec, "splitting batch")

	s.SetSplitThreshold(1 << 20) // no splits: renames land in existing shards
	rec.full, rec.entries = nil, nil
	rec.ids = putBatch(t, s, 12, 3) // new content, not a re-put
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	checkBarrier(t, rec, "non-splitting batch")
}

func checkBarrier(t *testing.T, rec *syncRecorder, batch string) {
	t.Helper()
	s := rec.s
	want := map[string]bool{}
	for _, p := range rec.finals() {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s: object not at its hash name after Sync: %s", batch, p)
		}
		for _, d := range dirsWithinRoot(s.root, filepath.Dir(p)) {
			want[d] = true
		}
	}
	got := map[string]bool{}
	for _, c := range rec.entries {
		got[c.dir] = true
		if !c.allPublished {
			t.Errorf("%s: entries of %s synced before every rename had landed", batch, c.dir)
		}
		if !within(s.root, c.dir) {
			t.Errorf("%s: synced a directory outside the store: %s", batch, c.dir)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s: synced %d directories, want %d", batch, len(got), len(want))
	}
	for d := range want {
		if !got[d] {
			t.Errorf("%s: directory not synced: %s", batch, d)
		}
	}
	if !want[s.root] {
		t.Fatalf("%s: the root must be among the directories to sync", batch)
	}
	if len(rec.full) != 2 || rec.full[0].allPublished || !rec.full[1].allPublished {
		t.Fatalf("%s: want one device flush before the renames and one after, got %+v", batch, rec.full)
	}
	if rec.full[0].dir != s.root || rec.full[1].dir != s.root {
		t.Fatalf("%s: device flushes are issued on the root, got %+v", batch, rec.full)
	}
}

// A root spelled with a trailing slash (as a config line may be) must
// not let the ancestor walk leave the store: the barrier would then open
// every ancestor up to the filesystem root, and fail on one the user
// cannot read. Measured on 1a4b1c9 with the parent at mode 0311.
func TestSyncNeverLeavesAnUncleanRoot(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions and a non-root user")
	}
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.MkdirAll(filepath.Join(parent, "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := NewTreeStore(filepath.Join(parent, "store") + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	if s.root != filepath.Join(parent, "store") {
		t.Fatalf("root must be cleaned, got %q", s.root)
	}
	s.SetSyncMode(SyncBatch)
	rec := &syncRecorder{s: s}
	rec.install(t)
	rec.ids = putBatch(t, s, 0, 3)
	if err := os.Chmod(parent, 0o311); err != nil { // traverse and write, no read
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })
	if err := s.Sync(); err != nil {
		t.Fatalf("Sync must not open directories above the store: %v", err)
	}
	for _, c := range append(rec.entries, rec.full...) {
		if !within(s.root, c.dir) {
			t.Fatalf("synced outside the store: %s", c.dir)
		}
	}
}

func TestDirsWithinRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "s", "store")
	got := dirsWithinRoot(root, filepath.Join(root, "c4", "ab", "cd"))
	want := []string{filepath.Join(root, "c4", "ab", "cd"), filepath.Join(root, "c4", "ab"), filepath.Join(root, "c4"), root}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if out := dirsWithinRoot(root, filepath.Join(string(filepath.Separator), "s", "elsewhere")); len(out) != 0 {
		t.Fatalf("a directory outside the root yields nothing, got %v", out)
	}
	if out := dirsWithinRoot(root, root); len(out) != 1 || out[0] != root {
		t.Fatalf("the root itself yields itself, got %v", out)
	}
}
