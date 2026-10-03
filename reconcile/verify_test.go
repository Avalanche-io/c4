package reconcile

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// streamSource is a ContentSource with no local path, so Apply must use
// the streaming copy. It serves whatever bytes it holds under an ID,
// tampered or not.
type streamSource map[c4.ID][]byte

func (s streamSource) Has(id c4.ID) bool { _, ok := s[id]; return ok }

func (s streamSource) Open(id c4.ID) (io.ReadCloser, error) {
	b, ok := s[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return ioutil.NopCloser(bytes.NewReader(b)), nil
}

// tamperedFolder returns a store.Folder holding evil bytes under the ID
// of good, and that ID.
func tamperedFolder(t *testing.T, good, evil string) (store.Folder, c4.ID) {
	t.Helper()
	dir := t.TempDir()
	id := c4.Identify(strings.NewReader(good))
	if err := os.WriteFile(filepath.Join(dir, id.String()), []byte(evil), 0644); err != nil {
		t.Fatal(err)
	}
	return store.Folder(dir), id
}

// applyOne plans and applies a single-file target at name in dstDir.
func applyOne(t *testing.T, r *Reconciler, dstDir, name, content string, id c4.ID) *Result {
	t.Helper()
	target := buildManifest(t, []testEntry{
		{name: name, content: content, id: id, mode: 0644},
	})
	plan, err := r.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Missing) > 0 {
		t.Fatalf("unexpected missing content: %v", plan.Missing)
	}
	res, err := r.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// assertRefused checks that Apply reported a mismatch naming id, left
// the destination file in its prior state, and left no temp files.
func assertRefused(t *testing.T, res *Result, id c4.ID, dstDir, name string, prior *string) {
	t.Helper()
	if res.Created != 0 {
		t.Errorf("Created = %d, want 0", res.Created)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("Errors = %v, want one mismatch error", res.Errors)
	}
	msg := res.Errors[0].Error()
	if !strings.Contains(msg, "does not match its ID") || !strings.Contains(msg, id.String()) {
		t.Errorf("error %q does not name the ID mismatch", msg)
	}
	got, err := os.ReadFile(filepath.Join(dstDir, name))
	switch {
	case prior == nil && err == nil:
		t.Errorf("destination file was created with %q", got)
	case prior != nil && string(got) != *prior:
		t.Errorf("destination file = %q, want untouched %q", got, *prior)
	}
	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp.") {
			t.Errorf("temp file %s left behind", e.Name())
		}
	}
}

// A tampered object in a local store (the file-to-file copy path) must
// not be materialized.
func TestApplyRefusesTamperedLocalStoreObject(t *testing.T) {
	for _, existing := range []bool{false, true} {
		src, id := tamperedFolder(t, "good content\n", "evil content\n")
		dst := t.TempDir()
		var prior *string
		if existing {
			s := "original\n"
			prior = &s
			if err := os.WriteFile(filepath.Join(dst, "a.txt"), []byte(s), 0644); err != nil {
				t.Fatal(err)
			}
		}
		res := applyOne(t, New(WithSource(src)), dst, "a.txt", "good content\n", id)
		assertRefused(t, res, id, dst, "a.txt", prior)
	}
}

// A tampered object from a streaming-only source must not be
// materialized, in serial and parallel batches alike.
func TestApplyRefusesTamperedStreamedObject(t *testing.T) {
	for _, workers := range []int{1, 4} {
		id := c4.Identify(strings.NewReader("good content\n"))
		src := streamSource{id: []byte("evil content\n")}
		dst := t.TempDir()
		r := New(WithSource(src), WithMaxConcurrency(workers))
		res := applyOne(t, r, dst, "a.txt", "good content\n", id)
		assertRefused(t, res, id, dst, "a.txt", nil)
	}
}

// A --source directory whose file changed after it was described must
// not supply the stale ID's content.
func TestApplyRefusesChangedDirSourceFile(t *testing.T) {
	srcDir := t.TempDir()
	id := writeFile(t, srcDir, "a.txt", "good content\n")
	m := buildManifest(t, []testEntry{
		{name: "a.txt", content: "good content\n", id: id, mode: 0644},
	})
	ds := NewDirSource(m, srcDir)
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("evil content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	res := applyOne(t, New(WithSource(ds)), dst, "a.txt", "good content\n", id)
	assertRefused(t, res, id, dst, "a.txt", nil)
}

// Intact content still materializes through both copy paths.
func TestApplyIntactContentUnchanged(t *testing.T) {
	const content = "good content\n"
	id := c4.Identify(strings.NewReader(content))

	folderDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(folderDir, id.String()), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	sources := map[string]ContentSource{
		"local":  store.Folder(folderDir),
		"stream": streamSource{id: []byte(content)},
	}
	for name, src := range sources {
		dst := t.TempDir()
		res := applyOne(t, New(WithSource(src)), dst, "a.txt", content, id)
		if len(res.Errors) != 0 || res.Created != 1 {
			t.Fatalf("%s: Created=%d Errors=%v", name, res.Created, res.Errors)
		}
		got, err := os.ReadFile(filepath.Join(dst, "a.txt"))
		if err != nil || string(got) != content {
			t.Fatalf("%s: got %q, %v", name, got, err)
		}
	}
}
