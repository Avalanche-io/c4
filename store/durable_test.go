package store

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestAtomicWriter(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "sub", "out.txt")

	w, err := NewAtomicWriter(final)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("atomic content")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "atomic content" {
		t.Fatalf("got %q", got)
	}

	// No temp files may remain.
	entries, err := os.ReadDir(filepath.Dir(final))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp.") {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestDurableWriterReadFrom(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("read from me"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, mk := range []func(string) (*DurableWriter, error){NewDurableWriter, NewAtomicWriter} {
		final := filepath.Join(dir, "dst.txt")
		w, err := mk(final)
		if err != nil {
			t.Fatal(err)
		}
		sf, err := os.Open(src)
		if err != nil {
			t.Fatal(err)
		}
		// io.Copy uses ReadFrom when available.
		if _, err := io.Copy(w, sf); err != nil {
			t.Fatal(err)
		}
		sf.Close()
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(final)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "read from me" {
			t.Fatalf("got %q", got)
		}
		os.Remove(final)
	}
}

func TestContentPath(t *testing.T) {
	content := []byte("content path test")
	id := c4.Identify(bytes.NewReader(content))

	type pather interface {
		ContentPath(c4.ID) (string, bool)
	}

	cases := []struct {
		name string
		mk   func(t *testing.T) Store
	}{
		{"Folder", func(t *testing.T) Store { return Folder(t.TempDir()) }},
		{"ShardedFolder", func(t *testing.T) Store { return ShardedFolder(t.TempDir()) }},
		{"TreeStore", func(t *testing.T) Store {
			s, err := NewTreeStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			return s
		}},
		{"MultiStore", func(t *testing.T) Store {
			return NewMultiStore(NewRAM(), Folder(t.TempDir()))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.mk(t)
			p, ok := s.(pather)
			if !ok {
				t.Fatalf("%s does not implement ContentPath", tc.name)
			}

			// Absent content: no path.
			if _, ok := p.ContentPath(id); ok {
				t.Fatal("expected no path before Put")
			}

			if _, err := s.Put(bytes.NewReader(content)); err != nil {
				t.Fatal(err)
			}

			// MultiStore writes to its first store (RAM, not local): the
			// path must still be unavailable rather than wrong.
			if tc.name == "MultiStore" {
				if _, ok := p.ContentPath(id); ok {
					t.Fatal("expected no local path for RAM-backed content")
				}
				return
			}

			path, ok := p.ContentPath(id)
			if !ok {
				t.Fatal("expected a path after Put")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, content) {
				t.Fatalf("path %s holds wrong content", path)
			}
		})
	}
}
