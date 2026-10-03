package reconcile

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
)

// backdateDirs sets every directory under root (root excluded) to a
// distinct time derived from base, deepest first so a parent's time is
// not disturbed by setting a child's.
func backdateDirs(t *testing.T, root string, base time.Time) {
	t.Helper()
	var dirs []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] < dirs[j] })
	for i, d := range dirs {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(d, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
}

// encode renders a manifest as canonical c4m text.
func encode(t *testing.T, m *c4m.Manifest) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c4m.NewEncoder(&buf).Encode(m); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestApplyNestedDirTimesSurviveWrites reconciles a tree whose
// directories already carry the target's timestamps — so Plan sees no
// directory metadata to fix — while files beneath them are replaced,
// added, removed, moved and relinked. Every one of those writes bumps
// its parent directory's mtime; after Apply each directory must still
// hold exactly the target's time, and a scan of dest must reproduce the
// target listing byte for byte.
func TestApplyNestedDirTimesSurviveWrites(t *testing.T) {
	nestedDirTimes(t, false)
}

// TestApplyNestedDirTimesSurviveSymlinkOps adds a symlink replacement to
// the same scenario. Symlink times themselves are not restorable with
// the standard library (Chtimes follows links), so only directory times
// are asserted here; the listing comparison lives in the test above.
func TestApplyNestedDirTimesSurviveSymlinkOps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	nestedDirTimes(t, true)
}

func nestedDirTimes(t *testing.T, withSymlink bool) {
	t.Helper()
	src := t.TempDir()
	dst := t.TempDir()
	fileTime := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	dirBase := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	common := map[string]string{
		"top.txt":           "top",
		"a/x.txt":           "x",
		"a/b/y.txt":         "y",
		"a/b/c/keep.txt":    "keep",
		"a/b/c/d/deep.txt":  "deep",
		"a/b/c/d/e/end.txt": "end",
		"logs/refs/heads/m": "ref",
	}
	targetOnly := map[string]string{
		"a/b/c/z.txt":        "z new",  // replaced
		"a/b/c/d/new.txt":    "added",  // added
		"a/b/c/d/e/mvd.txt":  "moving", // moved from a/b/c/mv.txt
		"logs/refs/heads/HE": "head 2", // replaced (git ref rewrite)
	}
	destOnly := map[string]string{
		"a/b/c/z.txt":        "z old",
		"a/b/c/d/gone.txt":   "removed",
		"a/b/c/mv.txt":       "moving",
		"logs/refs/heads/HE": "head 1",
	}
	write := func(root string, files map[string]string) {
		for name, content := range files {
			writeFile(t, root, filepath.FromSlash(name), content)
			p := filepath.Join(root, filepath.FromSlash(name))
			if err := os.Chtimes(p, fileTime, fileTime); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(src, common)
	write(src, targetOnly)
	write(dst, common)
	write(dst, destOnly)
	// An emptied subtree removed by rmdir, two levels down.
	if err := os.MkdirAll(filepath.Join(dst, "a", "b", "old", "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if withSymlink {
		if err := os.Symlink("../x.txt", filepath.Join(src, "a", "b", "c", "lnk")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("old-target", filepath.Join(dst, "a", "b", "c", "lnk")); err != nil {
			t.Fatal(err)
		}
	}
	// Shared directories in dst carry exactly the target's times, so the
	// plan has no directory timestamp to correct.
	backdateDirs(t, src, dirBase)
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || p == src {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		os.Chtimes(filepath.Join(dst, rel), info.ModTime(), info.ModTime())
		return nil
	})

	target, err := scan.Dir(src)
	if err != nil {
		t.Fatal(err)
	}
	want := encode(t, target)

	rec := New(WithSource(NewDirSource(target, src)))
	plan, err := rec.Plan(target, dst)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("plan missing %d IDs", len(plan.Missing))
	}
	res, err := rec.Apply(plan, dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("apply errors: %v", res.Errors)
	}

	// Every target directory carries exactly its recorded time.
	for rel, e := range c4m.EntryPaths(target.Entries) {
		if !e.IsDir() || e.Timestamp.Equal(c4m.NullTimestamp()) {
			continue
		}
		info, err := os.Stat(filepath.Join(dst, filepath.FromSlash(strings.TrimSuffix(rel, "/"))))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		if !info.ModTime().UTC().Truncate(time.Second).Equal(e.Timestamp.UTC()) {
			t.Errorf("%s mtime = %v, want %v", rel, info.ModTime().UTC(), e.Timestamp.UTC())
		}
	}

	if withSymlink {
		return
	}
	got, err := scan.Dir(dst)
	if err != nil {
		t.Fatal(err)
	}
	if g := encode(t, got); g != want {
		t.Fatalf("dest listing differs from target:\n--- want\n%s--- got\n%s", want, g)
	}
}
