package store

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Avalanche-io/c4"
)

// The Sink contract (store.go): Create's writer never names bytes that
// do not hash to the ID; Close of wrong bytes or of a write that
// stopped part way returns ErrInvalidID and leaves nothing named and
// nothing behind; Abort names nothing and the ID is creatable after;
// the right bytes are accepted and read back. Every Sink in this
// package runs through it, and TestEverySinkIsInTheContractTable fails
// when a type with a Create method is missing from the table below.

// sinkCase opens a fresh store and returns it with a root to check for
// leftover files (empty for stores that keep nothing on disk) and a
// function that prepares an ID before Create where the store needs it
// (MAP maps IDs to paths up front).
type sinkCase struct {
	name string
	open func(t *testing.T) (s Store, root string, prepare func(id c4.ID))
}

func sinkCases() []sinkCase {
	tree := func(mode SyncMode) func(t *testing.T) (Store, string, func(c4.ID)) {
		return func(t *testing.T) (Store, string, func(c4.ID)) {
			root := filepath.Join(t.TempDir(), "store")
			s, err := NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			s.SetSyncMode(mode)
			return s, root, nil
		}
	}
	return []sinkCase{
		{"TreeStore/each", tree(SyncEach)},
		{"TreeStore/batch", tree(SyncBatch)},
		{"TreeStore/none", tree(SyncNone)},
		{"Folder", func(t *testing.T) (Store, string, func(c4.ID)) {
			root := t.TempDir()
			return Folder(root), root, nil
		}},
		{"ShardedFolder", func(t *testing.T) (Store, string, func(c4.ID)) {
			root := t.TempDir()
			return ShardedFolder(root), root, nil
		}},
		{"MAP", func(t *testing.T) (Store, string, func(c4.ID)) {
			// MAP's Has answers from the map, which the caller fills before
			// Create, so "named" here is the file at the mapped path: the
			// contract test checks files under root for it.
			root := t.TempDir()
			m := make(map[c4.ID]string)
			return mapByFile{NewMap(m), m}, root, func(id c4.ID) { m[id] = filepath.Join(root, id.String()) }
		}},
		{"RAM", func(t *testing.T) (Store, string, func(c4.ID)) {
			return NewRAM(), "", nil
		}},
		{"S3Store", func(t *testing.T) (Store, string, func(c4.ID)) {
			srv := newContractBucket(t)
			s := NewS3Store("bucket", "c4/", "us-east-1", srv.URL, "AKID", "SECRET")
			return s, "", nil
		}},
		{"Validating/TreeStore", func(t *testing.T) (Store, string, func(c4.ID)) {
			s, root, _ := tree(SyncEach)(t)
			return NewValidating(s), root, nil
		}},
		{"Validating/foreign", func(t *testing.T) (Store, string, func(c4.ID)) {
			// A Sink from elsewhere that names on Close without a check:
			// Validating holds the contract on its behalf.
			root := t.TempDir()
			return NewValidating(unchecked{Folder(root)}), root, nil
		}},
		{"Logger/RAM", func(t *testing.T) (Store, string, func(c4.ID)) {
			return NewLogger(NewRAM(), io.Discard, LogCreate|LogClose|LogError), "", nil
		}},
		{"MultiStore/TreeStore", func(t *testing.T) (Store, string, func(c4.ID)) {
			s, root, _ := tree(SyncEach)(t)
			return NewMultiStore(s), root, nil
		}},
	}
}

// mapByFile makes a MAP's Has answer by the file at the mapped path,
// which is what "named" means for it in the contract test.
type mapByFile struct {
	MAP
	m map[c4.ID]string
}

func (s mapByFile) Has(id c4.ID) bool {
	p, ok := s.m[id]
	if !ok {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// unchecked is a Sink that trusts the caller's ID, as a store from
// outside this package might: its writer names whatever arrives.
type unchecked struct{ Folder }

func (u unchecked) Create(id c4.ID) (io.WriteCloser, error) {
	return NewDurableWriter(filepath.Join(string(u.Folder), id.String()))
}

func filesUnder(root string) []string {
	if root == "" {
		return nil
	}
	var out []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func syncIfBatch(s Store) {
	type syncer interface{ Sync() error }
	if ts, ok := s.(syncer); ok {
		ts.Sync()
	}
}

func TestSinkContract(t *testing.T) {
	for _, tc := range sinkCases() {
		t.Run(tc.name, func(t *testing.T) {
			s, root, prepare := tc.open(t)
			if prepare == nil {
				prepare = func(c4.ID) {}
			}
			right := bytes.Repeat([]byte("the bytes the ID names "), 700)
			id := c4.Identify(bytes.NewReader(right))
			prepare(id)

			// Wrong bytes: refused, nothing named, nothing left.
			w, err := s.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("other bytes entirely")); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("Close of wrong bytes returned %v, want ErrInvalidID", err)
			}
			syncIfBatch(s)
			if s.Has(id) {
				t.Fatal("wrong bytes were named under the ID")
			}
			if left := filesUnder(root); len(left) != 0 {
				t.Fatalf("wrong bytes left files behind: %v", left)
			}

			// A write that stopped part way, closed by a caller that does
			// not know to Abort: refused, nothing named.
			w, err = s.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			w.Write(right[:100])
			if err := w.Close(); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("Close of a partial write returned %v, want ErrInvalidID", err)
			}
			syncIfBatch(s)
			if s.Has(id) || len(filesUnder(root)) != 0 {
				t.Fatal("a partial write was named or left files behind")
			}

			// Abort: nothing named, nothing left, Close after is a no-op,
			// the ID is creatable again.
			w, err = s.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			w.Write(right[:100])
			a, ok := w.(Aborter)
			if !ok {
				t.Fatal("Create's writer must implement Aborter")
			}
			if err := a.Abort(); err != nil {
				t.Fatalf("Abort: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close after Abort returned %v", err)
			}
			syncIfBatch(s)
			if s.Has(id) || len(filesUnder(root)) != 0 {
				t.Fatal("an aborted write was named or left files behind")
			}

			// The writer says it checks, so Validating need not.
			c, ok := w.(Checked)
			if !ok || !c.ChecksID() {
				t.Fatal("Create's writer must report that it checks its bytes (Checked)")
			}

			// The right bytes: accepted and read back.
			w, err = s.Create(id)
			if err != nil {
				t.Fatalf("the ID must be creatable after a refusal and an abort: %v", err)
			}
			if _, err := w.Write(right); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close of the right bytes: %v", err)
			}
			syncIfBatch(s)
			if !s.Has(id) {
				t.Fatal("the right bytes must be named")
			}
			rc, err := s.Open(id)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(rc)
			rc.Close()
			if !bytes.Equal(got, right) {
				t.Fatalf("read back %d bytes, want %d", len(got), len(right))
			}
		})
	}
}

// Put of a source that fails part way names nothing and leaves nothing,
// on the store the CLI writes through. Also the evidence that the
// Create finding was never reachable from the CLI, which only Puts
// (measured the same on release/1.0.x, 2026-11-10).
func TestPutOfAFailingSourceNamesNothing(t *testing.T) {
	for _, mode := range []SyncMode{SyncEach, SyncBatch} {
		root := filepath.Join(t.TempDir(), "store")
		s, err := NewTreeStore(root)
		if err != nil {
			t.Fatal(err)
		}
		s.SetSyncMode(mode)
		whole := bytes.Repeat([]byte("the whole object "), 1000)
		if _, err := s.Put(&failingReader{bytes.NewReader(whole), 100}); err == nil {
			t.Fatal("Put of a failing source must fail")
		}
		s.Sync()
		if s.Has(c4.Identify(bytes.NewReader(whole))) || s.Has(c4.Identify(bytes.NewReader(whole[:100]))) {
			t.Fatal("a failed Put must name neither the object nor the partial bytes")
		}
		if left := filesUnder(root); len(left) != 0 {
			t.Fatalf("a failed Put must leave no file, found %v", left)
		}
	}
}

// Every type in this package with a Create(id c4.ID) method is in the
// contract table above, so a new Sink cannot be added without running
// through it. Wrappers are listed under the store they wrap.
func TestEverySinkIsInTheContractTable(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var creators []string
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Name.Name != "Create" || fn.Recv == nil || fn.Type.Params.NumFields() != 1 {
					continue
				}
				recv := fn.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				creators = append(creators, recv.(*ast.Ident).Name)
			}
		}
	}
	sort.Strings(creators)
	covered := map[string]bool{}
	for _, tc := range sinkCases() {
		covered[strings.SplitN(tc.name, "/", 2)[0]] = true
	}
	var missing []string
	for _, c := range creators {
		if !covered[c] {
			missing = append(missing, c)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("Sinks with no row in sinkCases (the contract table): %v; every Create must run through TestSinkContract", missing)
	}
	if len(creators) < 8 {
		t.Fatalf("found only %d Create methods, the parser lost some: %v", len(creators), creators)
	}
}

// contractBucket is an in-memory bucket for the contract test: PUT stores, HEAD
// and GET serve, DELETE removes. Signatures are not checked.
type contractBucket struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newContractBucket(t *testing.T) *httptest.Server {
	f := &contractBucket{objects: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case "PUT":
			body, _ := io.ReadAll(r.Body)
			f.objects[r.URL.Path] = body
		case "HEAD", "GET":
			body, ok := f.objects[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			if r.Method == "GET" {
				w.Write(body)
			}
		case "DELETE":
			delete(f.objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// BenchmarkValidatingCreateOverTreeStore measures a Create through
// Validating over a TreeStore: before the Checked pass-through the bytes
// were hashed twice, once by each layer.
func BenchmarkValidatingCreateOverTreeStore(b *testing.B) {
	root := filepath.Join(b.TempDir(), "store")
	ts, err := NewTreeStore(root)
	if err != nil {
		b.Fatal(err)
	}
	ts.SetSyncMode(SyncNone)
	v := NewValidating(ts)
	data := bytes.Repeat([]byte("benchmark object bytes "), 1<<20) // ~23 MiB
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		obj := append([]byte(fmt.Sprintf("%d:", i)), data...)
		id := c4.Identify(bytes.NewReader(obj))
		b.StartTimer()
		w, err := v.Create(id)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := w.Write(obj); err != nil {
			b.Fatal(err)
		}
		if err := w.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
