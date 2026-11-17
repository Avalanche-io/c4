package reconcile

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// movingStore is a tree store whose object another process's split moves
// one level down right after ContentPath has looked, before the
// materialiser opens the path it returned.
type movingStore struct {
	*store.TreeStore
	moved, opens int
}

func (m *movingStore) ContentPath(id c4.ID) (string, bool) {
	p, ok := m.TreeStore.ContentPath(id)
	if ok && m.moved == 0 {
		m.moved++
		str := id.String()
		sub := filepath.Join(m.Root(), str[0:2])
		if err := os.MkdirAll(sub, 0o755); err == nil {
			os.Rename(p, filepath.Join(sub, str)) // the split's rename
		}
	}
	return p, ok
}

func (m *movingStore) Open(id c4.ID) (io.ReadCloser, error) {
	m.opens++
	return m.TreeStore.Open(id)
}

// Apply's create takes the local path a store's ContentPath gives and
// copies from it (copyFile: open, then a verified copy). A split in
// another process can move the object between the two. The create must
// still materialise the right bytes. What carries it is the fallback that
// follows a failed local copy: the stream from the store's Open, which
// looks again (and, since 2026-11-16, follows a move during its own open).
//
// This is a pin, not a fix: it passes on release/1.0.27 before this
// change and on master b82ec07 too (measured 2026-11-16). A second look inside copyFile would duplicate the
// fallback for nothing: with a checked ID the local copy is os.Open and a
// hashing io.Copy, the same work as the stream. A MultiStore passes its
// member's path through and its Open goes to the member, so it needs
// nothing either. Without the fallback this fails.
func TestApplyCreateSurvivesAMoveAfterContentPath(t *testing.T) {
	storeDir := t.TempDir()
	ts, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	content := "bytes a split moves between the look and the copy\n"
	id, err := ts.Put(bytes.NewReader([]byte(content)))
	if err != nil {
		t.Fatal(err)
	}
	ms := &movingStore{TreeStore: ts}
	target := buildManifest(t, []testEntry{{name: "a.txt", content: content, id: id, mode: 0o644}})
	dst := t.TempDir()
	rec := New(WithSource(ms))
	plan, err := rec.Plan(target, dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Apply(plan, dst); err != nil {
		t.Fatalf("Apply after the object moved: %v", err)
	}
	if ms.moved != 1 {
		t.Fatal("fixture: the move did not happen")
	}
	got, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil || string(got) != content {
		t.Fatalf("materialised %q, %v; want the stored bytes", got, err)
	}
	t.Logf("local copy failed on the moved path; the store's Open was used %d time(s)", ms.opens)
}
