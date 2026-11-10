package store

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// leftovers lists every file under root, so a test can assert that an
// abandoned write left neither an object nor a temp file.
func leftovers(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

var treeModes = []struct {
	name string
	mode SyncMode
}{{"each", SyncEach}, {"batch", SyncBatch}, {"none", SyncNone}}

// Abort discards a write in every sync mode: nothing is named, no temp
// file stays, Close afterwards is a no-op, and the ID can be created
// again.
func TestCreateAbortNamesNothingAndLeavesNoFile(t *testing.T) {
	for _, m := range treeModes {
		t.Run(m.name, func(t *testing.T) {
			s, err := NewTreeStore(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			s.SetSyncMode(m.mode)
			whole := bytes.Repeat([]byte("object "), 500)
			id := c4.Identify(bytes.NewReader(whole))
			w, err := s.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(whole[:50]); err != nil {
				t.Fatal(err)
			}
			a, ok := w.(Aborter)
			if !ok {
				t.Fatal("Create's writer must implement Aborter")
			}
			if err := a.Abort(); err != nil {
				t.Fatalf("Abort: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close after Abort must be a no-op, got %v", err)
			}
			if err := a.Abort(); err != nil {
				t.Fatalf("second Abort must be a no-op, got %v", err)
			}
			if err := s.Sync(); err != nil {
				t.Fatal(err)
			}
			if s.Has(id) {
				t.Fatal("an aborted write must not be named")
			}
			if left := leftovers(t, s.root); len(left) != 0 {
				t.Fatalf("an aborted write must leave no file, found %v", left)
			}
			again, err := s.Create(id)
			if err != nil {
				t.Fatalf("the ID must be creatable again after Abort: %v", err)
			}
			again.(Aborter).Abort() // an open temp file blocks TempDir cleanup on Windows
		})
	}
}

// Close checks the bytes against the ID the caller chose: wrong bytes,
// or a write that stopped part way, are refused with ErrInvalidID,
// nothing is named and no temp file stays; the ID can be created again.
func TestCreateCloseRefusesBytesThatDoNotHashToTheID(t *testing.T) {
	for _, m := range treeModes {
		t.Run(m.name, func(t *testing.T) {
			s, err := NewTreeStore(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			s.SetSyncMode(m.mode)
			id := c4.Identify(strings.NewReader("the bytes the ID names"))
			w, err := s.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("other bytes")); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("Close of the wrong bytes returned %v, want ErrInvalidID", err)
			}
			if err := s.Sync(); err != nil {
				t.Fatal(err)
			}
			if s.Has(id) {
				t.Fatal("wrong bytes must not be named under the ID")
			}
			if left := leftovers(t, s.root); len(left) != 0 {
				t.Fatalf("a refused write must leave no file, found %v", left)
			}
			w, err = s.Create(id)
			if err != nil {
				t.Fatalf("the ID must be creatable again after a refusal: %v", err)
			}
			if _, err := io.WriteString(w, "the bytes the ID names"); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("the right bytes must close cleanly: %v", err)
			}
			if err := s.Sync(); err != nil {
				t.Fatal(err)
			}
			if !s.Has(id) {
				t.Fatal("the right bytes must be named")
			}
		})
	}
}

// cannotRemove is a store whose Remove is unsupported, as the Store
// interface allows; Validating over it must still name nothing when it
// refuses a write.
type cannotRemove struct{ Store }

func (cannotRemove) Remove(c4.ID) error { return ErrNotImplemented }

// Validating checks before the inner writer closes. Over a TreeStore it
// aborts the inner write; over a Folder (no Abort of its own on the
// interface, though its DurableWriter has one) the same; over a store
// that cannot remove, nothing is named either way.
func TestValidatingChecksBeforeTheInnerWriterNames(t *testing.T) {
	stores := map[string]func(t *testing.T) (Store, string){
		"tree": func(t *testing.T) (Store, string) {
			root := filepath.Join(t.TempDir(), "store")
			s, err := NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			return s, root
		},
		"folder": func(t *testing.T) (Store, string) {
			root := t.TempDir()
			return Folder(root), root
		},
	}
	for name, open := range stores {
		t.Run(name, func(t *testing.T) {
			inner, root := open(t)
			v := NewValidating(cannotRemove{inner})
			id := c4.Identify(strings.NewReader("right"))
			w, err := v.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(w, "wrong"); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("Close returned %v, want ErrInvalidID", err)
			}
			if inner.Has(id) {
				t.Fatal("refused bytes must not be in the store under the ID")
			}
			if left := leftovers(t, root); len(left) != 0 {
				t.Fatalf("a refused write must leave no file, found %v", left)
			}
			// Abort passes through.
			w, err = v.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			io.WriteString(w, "ri")
			if err := w.(Aborter).Abort(); err != nil {
				t.Fatal(err)
			}
			if inner.Has(id) || len(leftovers(t, root)) != 0 {
				t.Fatal("an aborted validating write must leave nothing")
			}
		})
	}
}

// failingReader fails after a prefix, as a network source does.
type failingReader struct {
	r    io.Reader
	left int
}

func (f *failingReader) Read(b []byte) (int, error) {
	if f.left <= 0 {
		return 0, errors.New("source failed")
	}
	if len(b) > f.left {
		b = b[:f.left]
	}
	n, err := f.r.Read(b)
	f.left -= n
	return n, err
}

// A failed Put (Folder, Map, RAM and Sharded share defaultPut) leaves
// nothing behind, and the writer defaultPut gives up on a copy error is
// aborted where it can be: a DurableWriter's temp file is removed and
// its final path never written.
func TestDefaultPutLeavesNothingWhenTheSourceFails(t *testing.T) {
	root := t.TempDir()
	f := Folder(root)
	whole := bytes.Repeat([]byte("put object "), 300)
	if _, err := f.Put(&failingReader{bytes.NewReader(whole), 100}); err == nil {
		t.Fatal("Put of a failing source must fail")
	}
	if left := leftovers(t, root); len(left) != 0 {
		t.Fatalf("a failed Put must leave no file, found %v", left)
	}

	final := filepath.Join(root, "abandoned")
	w, err := NewDurableWriter(final)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(w, "part")
	if err := abortOrClose(w); err != nil {
		t.Fatal(err)
	}
	if left := leftovers(t, root); len(left) != 0 {
		t.Fatalf("abortOrClose on a DurableWriter must remove its temp file and write nothing, found %v", left)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close after Abort must be a no-op, got %v", err)
	}
}
