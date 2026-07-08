package scan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// emptyInputID is the C4 ID of zero bytes of input — the ID that buggy
// guided scans used to assign to every directory.
func emptyInputID() c4.ID {
	return c4.Identify(bytes.NewReader(nil))
}

// buildMixedTree creates a small tree with nesting and a same-named file in
// two different directories.
func buildMixedTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "top.txt"), []byte("top"), 0644))
	must(t, os.MkdirAll(filepath.Join(dir, "x", "inner"), 0755))
	must(t, os.WriteFile(filepath.Join(dir, "x", "f.txt"), []byte("x-content"), 0644))
	must(t, os.WriteFile(filepath.Join(dir, "x", "inner", "deep.txt"), []byte("deep"), 0644))
	must(t, os.MkdirAll(filepath.Join(dir, "y"), 0755))
	must(t, os.WriteFile(filepath.Join(dir, "y", "f.txt"), []byte("y-content"), 0644))
	return dir
}

// TestGuidedScanDirectoryIDs is the regression test for the guided-scan
// corruption: `c4 id --continue a.c4m dir` used to write the empty-input ID
// for every directory because the per-directory sub-scan re-rooted the walk
// while the guide paths stayed root-relative. A guided re-scan of an
// unchanged tree must reproduce the original manifest exactly.
func TestGuidedScanDirectoryIDs(t *testing.T) {
	dir := buildMixedTree(t)

	original, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}

	guided, err := Dir(dir, WithGuide(original))
	if err != nil {
		t.Fatal(err)
	}

	if len(guided.Entries) != len(original.Entries) {
		t.Fatalf("guided scan entry count = %d, want %d", len(guided.Entries), len(original.Entries))
	}

	empty := emptyInputID()
	for i, want := range original.Entries {
		got := guided.Entries[i]
		if got.IsDir() && got.C4ID == empty {
			t.Errorf("directory %q has the empty-input ID — guided-scan corruption", got.Name)
		}
		if got.Canonical() != want.Canonical() {
			t.Errorf("entry %d differs:\n  guided:   %s\n  original: %s", i, got.Canonical(), want.Canonical())
		}
	}
}

// TestDirectoryIDMatchesFreshScan verifies the spec's merkle property: every
// directory entry's C4 ID equals the ID a fresh scan rooted at that
// directory would produce. This pins the bottom-up computation to the same
// bytes as Manifest.ComputeC4ID over the directory's own manifest.
func TestDirectoryIDMatchesFreshScan(t *testing.T) {
	dir := buildMixedTree(t)

	m, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}

	dirs := 0
	for _, e := range m.Entries {
		if !e.IsDir() {
			continue
		}
		dirs++
		sub := filepath.Join(dir, filepath.FromSlash(m.EntryPath(e)))
		fresh, err := Dir(sub)
		if err != nil {
			t.Fatalf("fresh scan of %s: %v", sub, err)
		}
		if want := fresh.ComputeC4ID(); e.C4ID != want {
			t.Errorf("dir %q ID mismatch:\n  walk:  %s\n  fresh: %s", m.EntryPath(e), e.C4ID, want)
		}
	}
	if dirs == 0 {
		t.Fatal("test tree contained no directories")
	}
}

// TestDeepTreeLinear is the regression test for the O(2^depth) directory-ID
// computation. The old implementation re-scanned every directory's full
// subtree recursively, so a 40-deep chain required ~2^40 directory walks and
// would never finish. Bottom-up computation completes in milliseconds. The
// context timeout turns a complexity regression into a clean test failure.
func TestDeepTreeLinear(t *testing.T) {
	const depth = 40
	root := t.TempDir()
	p := root
	for i := 1; i <= depth; i++ {
		p = filepath.Join(p, fmt.Sprintf("d%02d", i))
	}
	must(t, os.MkdirAll(p, 0755))
	must(t, os.WriteFile(filepath.Join(p, "leaf.txt"), []byte("leaf"), 0644))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	m, err := Dir(root, WithContext(ctx))
	if err != nil {
		t.Fatalf("deep scan failed (complexity regression?) after %v: %v", time.Since(start), err)
	}

	if len(m.Entries) != depth+1 {
		t.Fatalf("entry count = %d, want %d", len(m.Entries), depth+1)
	}
	empty := emptyInputID()
	for _, e := range m.Entries {
		if !e.IsDir() {
			continue
		}
		if e.C4ID.IsNil() {
			t.Errorf("dir %q has nil C4 ID", e.Name)
		}
		if e.C4ID == empty {
			t.Errorf("dir %q has the empty-input ID", e.Name)
		}
	}

	// Spot-check the merkle chain: the deepest directory's ID must match a
	// fresh scan rooted there.
	fresh, err := Dir(p)
	if err != nil {
		t.Fatal(err)
	}
	var deepest *c4m.Entry
	for _, e := range m.Entries {
		if e.IsDir() && e.Depth == depth-1 {
			deepest = e
		}
	}
	if deepest == nil {
		t.Fatalf("no directory entry at depth %d", depth-1)
	}
	if want := fresh.ComputeC4ID(); deepest.C4ID != want {
		t.Errorf("deepest dir ID mismatch:\n  walk:  %s\n  fresh: %s", deepest.C4ID, want)
	}
}

// TestStructureModeNullTimestamp verifies that structure-mode scans render
// null timestamps as "-" per the spec, not as the zero time's
// 0001-01-01T00:00:00Z.
func TestStructureModeNullTimestamp(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644))
	must(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
	must(t, os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0644))

	m, err := Dir(dir, WithMode(ModeStructure))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) == 0 {
		t.Fatal("no entries")
	}

	for _, e := range m.Entries {
		if !e.Timestamp.Equal(c4m.NullTimestamp()) {
			t.Errorf("%q: timestamp = %v, want null sentinel", e.Name, e.Timestamp)
		}
		line := e.Canonical()
		if strings.Contains(line, "0001-01-01") {
			t.Errorf("%q renders zero time instead of '-': %s", e.Name, line)
		}
	}

	// The exact canonical form of a structure-mode file entry.
	for _, e := range m.Entries {
		if e.Name == "a.txt" {
			if got, want := e.Canonical(), "- - - a.txt -"; got != want {
				t.Errorf("canonical = %q, want %q", got, want)
			}
		}
	}
}

// BenchmarkDirIDsByDepth measures full-mode scans of single-file chains at
// increasing depth. Time must grow linearly with depth (the old top-down
// implementation doubled per level).
func BenchmarkDirIDsByDepth(b *testing.B) {
	for _, depth := range []int{4, 8, 12, 16} {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			root := b.TempDir()
			p := root
			for i := 1; i <= depth; i++ {
				p = filepath.Join(p, fmt.Sprintf("d%02d", i))
			}
			if err := os.MkdirAll(p, 0755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, "leaf.txt"), []byte("leaf"), 0644); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Dir(root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
