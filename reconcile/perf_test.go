package reconcile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

type treeFile struct {
	rel     string // slash-separated path relative to the tree root
	content string
	id      c4.ID
}

// buildTree writes count files under dir (10 per subdirectory) and returns
// the matching target manifest plus the file list.
func buildTree(t *testing.T, dir string, count int) (*c4m.Manifest, []treeFile) {
	t.Helper()
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	m := c4m.NewManifest()
	var files []treeFile
	for d := 0; d*10 < count; d++ {
		sub := fmt.Sprintf("d%02d", d)
		m.AddEntry(&c4m.Entry{Name: sub + "/", Mode: os.ModeDir | 0755, Size: 0, Timestamp: ts, Depth: 0})
		for f := 0; f < 10 && d*10+f < count; f++ {
			// Duplicate content every 7th file to exercise shared IDs.
			content := fmt.Sprintf("content-%d", d*10+f)
			if (d*10+f)%7 == 0 {
				content = "content-dup"
			}
			name := fmt.Sprintf("f%02d.txt", f)
			rel := sub + "/" + name
			id := writeFile(t, dir, rel, content)
			m.AddEntry(&c4m.Entry{Name: name, Mode: 0644, Size: int64(len(content)), C4ID: id, Timestamp: ts, Depth: 1})
			files = append(files, treeFile{rel: rel, content: content, id: id})
		}
	}
	return m, files
}

func TestApplyParallelCreates(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	target, files := buildTree(t, srcDir, 120)

	rec := New(
		WithSource(NewDirSource(target, srcDir)),
		WithMaxConcurrency(8),
		WithSyncMode(store.SyncNone),
	)
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("expected complete plan, got %d missing", len(plan.Missing))
	}

	res, err := rec.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if res.Created != 120+12 { // 120 files + 12 directories
		t.Fatalf("expected 132 created, got %d", res.Created)
	}

	// Verify every file materialized with the right content.
	for _, f := range files {
		got, err := os.ReadFile(filepath.Join(dstDir, filepath.FromSlash(f.rel)))
		if err != nil {
			t.Fatalf("%s: %v", f.rel, err)
		}
		if string(got) != f.content {
			t.Fatalf("%s: content mismatch", f.rel)
		}
	}

	// No temp files may remain.
	filepath.Walk(dstDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasPrefix(filepath.Base(path), ".tmp.") {
			t.Fatalf("leftover temp file: %s", path)
		}
		return nil
	})
}

// failOpenSource claims to have every ID but fails Open for chosen IDs.
type failOpenSource struct {
	inner ContentSource
	fail  map[c4.ID]bool
}

func (s *failOpenSource) Has(id c4.ID) bool { return true }

func (s *failOpenSource) Open(id c4.ID) (io.ReadCloser, error) {
	if s.fail[id] {
		return nil, fmt.Errorf("refused %s", id)
	}
	return s.inner.Open(id)
}

func TestApplyParallelErrorOrder(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	target, files := buildTree(t, srcDir, 40)

	// Fail every entry whose content ends in "3" (unique, non-dup IDs).
	fail := make(map[c4.ID]bool)
	var failNames []string
	for _, f := range files {
		if !strings.HasSuffix(f.content, "3") {
			continue
		}
		fail[f.id] = true
		failNames = append(failNames, f.rel)
	}
	if len(failNames) < 2 {
		t.Fatal("test needs at least two failing entries")
	}

	rec := New(
		WithSource(&failOpenSource{inner: NewDirSource(target, srcDir), fail: fail}),
		WithMaxConcurrency(8),
		WithSyncMode(store.SyncNone),
	)
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != len(failNames) {
		t.Fatalf("expected %d errors, got %d: %v", len(failNames), len(res.Errors), res.Errors)
	}

	// Errors must appear in operation (plan) order: find each failing path
	// in plan order and match against reported errors.
	var wantOrder []string
	for _, op := range plan.Operations {
		if op.Type == OpCreate && fail[op.ContentID] {
			wantOrder = append(wantOrder, op.Path)
		}
	}
	for i, e := range res.Errors {
		if !strings.Contains(e.Error(), wantOrder[i]) {
			t.Fatalf("error %d = %q, expected path %q", i, e.Error(), wantOrder[i])
		}
	}
}

func TestApplySequentialMatchesParallel(t *testing.T) {
	srcDir := t.TempDir()
	target, _ := buildTree(t, srcDir, 50)

	for _, workers := range []int{1, 4} {
		dstDir := t.TempDir()
		rec := New(
			WithSource(NewDirSource(target, srcDir)),
			WithMaxConcurrency(workers),
		)
		plan, err := rec.Plan(target, dstDir)
		if err != nil {
			t.Fatal(err)
		}
		res, err := rec.Apply(plan, dstDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Errors) != 0 || res.Created != 55 {
			t.Fatalf("workers=%d: created=%d errors=%v", workers, res.Created, res.Errors)
		}
	}
}

// trustedFixture writes a destination file whose content differs from the
// manifest ("aleph" vs "alpha") while size matches, plus a source dir
// holding the real "alpha" content. Returns the target manifest and dir.
func trustedFixture(t *testing.T, mtime time.Time) (*c4m.Manifest, *DirSource, string) {
	t.Helper()
	dir := t.TempDir()
	srcDir := t.TempDir()

	idAlpha := writeFile(t, srcDir, "a.txt", "alpha")
	writeFile(t, dir, "a.txt", "aleph")
	os.Chtimes(filepath.Join(dir, "a.txt"), mtime, mtime)

	target := buildManifest(t, []testEntry{
		{name: "a.txt", content: "alpha", id: idAlpha, mode: 0644},
	})
	return target, NewDirSource(target, srcDir), dir
}

func TestPlanTrustedMetadata(t *testing.T) {
	// buildManifest stamps entries with this timestamp.
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	target, src, dir := trustedFixture(t, ts)

	// Default: the file is hashed, the mismatch is detected.
	plan, err := New(WithSource(src)).Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("expected complete plan, missing %d", len(plan.Missing))
	}
	creates := 0
	for _, op := range plan.Operations {
		if op.Type == OpCreate {
			creates++
		}
	}
	if creates != 1 {
		t.Fatalf("default plan: expected 1 create, got %d", creates)
	}

	// Trusted metadata: size+mtime match, so the file is assumed current.
	plan, err = New(WithSource(src), WithTrustedMetadata(true)).Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("expected complete plan, missing %d", len(plan.Missing))
	}
	for _, op := range plan.Operations {
		if op.Type == OpCreate {
			t.Fatalf("trusted plan: unexpected create for %s", op.Path)
		}
	}
}

func TestPlanTrustedMetadataMtimeMismatch(t *testing.T) {
	// Same size, different mtime: trust must not apply, hash reveals drift.
	other := time.Date(2020, 6, 6, 6, 0, 0, 0, time.UTC)
	target, src, dir := trustedFixture(t, other)

	plan, err := New(WithSource(src), WithTrustedMetadata(true)).Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("expected complete plan, missing %d", len(plan.Missing))
	}
	creates := 0
	for _, op := range plan.Operations {
		if op.Type == OpCreate {
			creates++
		}
	}
	if creates != 1 {
		t.Fatalf("expected 1 create, got %d", creates)
	}
}

// pathSource serves content from an explicit id -> path map via both the
// streaming and local-path interfaces.
type pathSource struct {
	paths map[c4.ID]string
}

func (s *pathSource) Has(id c4.ID) bool { _, ok := s.paths[id]; return ok }

func (s *pathSource) Open(id c4.ID) (io.ReadCloser, error) {
	p, ok := s.paths[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return os.Open(p)
}

func (s *pathSource) ContentPath(id c4.ID) (string, bool) {
	p, ok := s.paths[id]
	return p, ok
}

func TestApplyLocalSourceFastPath(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	id := writeFile(t, srcDir, "blob", "local-content")
	target := buildManifest(t, []testEntry{
		{name: "a.txt", content: "local-content", id: id, mode: 0644},
	})

	src := &pathSource{paths: map[c4.ID]string{id: filepath.Join(srcDir, "blob")}}
	rec := New(WithSource(src), WithSyncMode(store.SyncNone))
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || res.Created != 1 {
		t.Fatalf("created=%d errors=%v", res.Created, res.Errors)
	}
	got, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	if err != nil || string(got) != "local-content" {
		t.Fatalf("got %q, %v", got, err)
	}
}

// lyingPathSource reports a nonexistent local path but streams correctly,
// exercising the fallback from the local fast path to Open.
type lyingPathSource struct {
	pathSource
}

func (s *lyingPathSource) ContentPath(id c4.ID) (string, bool) {
	return filepath.Join(os.TempDir(), "does-not-exist-c4-test"), true
}

func TestApplyLocalSourceFallback(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	id := writeFile(t, srcDir, "blob", "fallback-content")
	target := buildManifest(t, []testEntry{
		{name: "a.txt", content: "fallback-content", id: id, mode: 0644},
	})

	src := &lyingPathSource{pathSource{paths: map[c4.ID]string{id: filepath.Join(srcDir, "blob")}}}
	rec := New(WithSource(src))
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || res.Created != 1 {
		t.Fatalf("created=%d errors=%v", res.Created, res.Errors)
	}
	got, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	if err != nil || string(got) != "fallback-content" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDirSourceContentPath(t *testing.T) {
	dir := t.TempDir()
	id := writeFile(t, dir, "a.txt", "alpha")

	m := buildManifest(t, []testEntry{
		{name: "a.txt", content: "alpha", id: id, mode: 0644},
	})
	ds := NewDirSource(m, dir)

	p, ok := ds.ContentPath(id)
	if !ok || p != filepath.Join(dir, "a.txt") {
		t.Fatalf("got %q, %v", p, ok)
	}

	// Remove the file: ContentPath must report unavailable.
	os.Remove(filepath.Join(dir, "a.txt"))
	if _, ok := ds.ContentPath(id); ok {
		t.Fatal("expected no path after removal")
	}

	var missing c4.ID
	copy(missing[:], bytes.Repeat([]byte{1}, 64))
	if _, ok := ds.ContentPath(missing); ok {
		t.Fatal("expected no path for unknown id")
	}
}
