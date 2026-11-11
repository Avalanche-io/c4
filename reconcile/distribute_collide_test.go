package reconcile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A target that already holds an entry under another spelling (Readme.md,
// Docs/) is written through a folding lookup, which keeps the old name:
// after Distribute every written entry is listed under exactly its source
// name, with the source's bytes.
func TestDistributeRespellsWhatTheTargetHeldUnderAnotherSpelling(t *testing.T) {
	dst := t.TempDir()
	if !pairFolds(t, dst, "Readme.md", "readme.md") {
		t.Skip("this volume tells Readme.md and readme.md apart")
	}
	os.WriteFile(filepath.Join(dst, "Readme.md"), []byte("old\n"), 0o644)
	os.MkdirAll(filepath.Join(dst, "Docs"), 0o755)
	os.WriteFile(filepath.Join(dst, "Docs", "A.txt"), []byte("old\n"), 0o644)

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "readme.md"), []byte("new\n"), 0o644)
	os.MkdirAll(filepath.Join(src, "docs"), 0o755)
	os.WriteFile(filepath.Join(src, "docs", "a.txt"), []byte("new a\n"), 0o644)

	res, err := Distribute(src, ToDir(dst))
	if err != nil {
		t.Fatal(err)
	}
	if errs := res.Targets[0].Errors; len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if got := listing(t, dst); !reflect.DeepEqual(got, []string{"docs/", "docs/a.txt", "readme.md"}) {
		t.Fatalf("target lists %q, want exactly the source's names", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "docs", "a.txt")); string(b) != "new a\n" {
		t.Errorf("docs/a.txt = %q, want the source's bytes", b)
	}
}

// A source on a case-sensitive volume may hold names a folding target
// takes for one. Each is measured at the target before its directory's
// children are written: the byte-order last is written whole, the others
// are refused with their subtrees and reported, and a refused directory's
// time is never set onto the winner. Before, the second write went through
// the first's name (one file, the other's bytes; two directories merged).
//
// It needs a source that holds both spellings: set C4_TEST_CASE_SENSITIVE_DIR
// to a directory on a case-sensitive volume.
func TestDistributeRefusesNamesTheTargetFolds(t *testing.T) {
	base := os.Getenv("C4_TEST_CASE_SENSITIVE_DIR")
	if base == "" {
		t.Skip("set C4_TEST_CASE_SENSITIVE_DIR to a directory on a case-sensitive volume")
	}
	src, err := os.MkdirTemp(base, "dist-src-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(src)
	if pairFolds(t, src, "README.md", "readme.md") {
		t.Skip("C4_TEST_CASE_SENSITIVE_DIR folds case")
	}
	dst := t.TempDir()
	if !pairFolds(t, dst, "README.md", "readme.md") {
		t.Skip("the target volume tells README.md and readme.md apart")
	}
	os.WriteFile(filepath.Join(src, "README.md"), []byte("upper\n"), 0o644)
	os.WriteFile(filepath.Join(src, "readme.md"), []byte("lower\n"), 0o644)
	os.MkdirAll(filepath.Join(src, "Docs"), 0o755)
	os.WriteFile(filepath.Join(src, "Docs", "x.txt"), []byte("upper x\n"), 0o644)
	os.WriteFile(filepath.Join(src, "Docs", "y.txt"), []byte("upper y\n"), 0o644)
	os.MkdirAll(filepath.Join(src, "docs"), 0o755)
	os.WriteFile(filepath.Join(src, "docs", "x.txt"), []byte("lower x\n"), 0o644)

	res, err := Distribute(src, ToDir(dst))
	if err != nil {
		t.Fatal(err)
	}
	tr := res.Targets[0]
	if len(tr.Errors) > 0 {
		t.Fatalf("errors: %v", tr.Errors)
	}
	if got := listing(t, dst); !reflect.DeepEqual(got, []string{"docs/", "docs/x.txt", "readme.md"}) {
		t.Fatalf("target lists %q, want the byte-order last of each set", got)
	}
	for rel, want := range map[string]string{"readme.md": "lower\n", "docs/x.txt": "lower x\n"} {
		if b, _ := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel))); string(b) != want {
			t.Errorf("%s = %q, want %q", rel, b, want)
		}
	}
	want := []Refusal{
		{Path: "Docs/", Reason: "this folder takes it for docs (measured)", Entries: 3},
		{Path: "README.md", Reason: "this folder takes it for readme.md (measured)", Entries: 1},
	}
	if !reflect.DeepEqual(tr.Refused, want) {
		t.Errorf("refused %+v, want %+v", tr.Refused, want)
	}
	// The manifest still describes the whole source.
	if n := len(res.Manifest.Entries); n != 7 {
		t.Errorf("manifest has %d entries, want all 7 of the source", n)
	}
}
