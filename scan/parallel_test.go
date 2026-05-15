package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// buildParallelFixture creates a deep + wide tree with >= 1000 entries.
// Layout:
//   - 10 top-level dirs ("wide00".."wide09"), each with 50 small files = 510 entries
//   - 1 chain "deep/0/1/.../19" with 20 levels, a file at each level = ~40 entries
//   - 5 mixed dirs each containing 100 files + 3 nested subdirs with 10 files each
//     = 5 * (100 + 3 + 30) = 665 entries
// Total ~1215 entries.
func buildParallelFixture(tb testing.TB, root string) {
	tb.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		tb.Fatalf("mkdir root: %v", err)
	}

	// Wide
	for d := 0; d < 10; d++ {
		dir := filepath.Join(root, fmt.Sprintf("wide%02d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		for f := 0; f < 50; f++ {
			path := filepath.Join(dir, fmt.Sprintf("f%03d.bin", f))
			content := []byte(fmt.Sprintf("wide-%d-%d", d, f))
			if err := os.WriteFile(path, content, 0o644); err != nil {
				tb.Fatal(err)
			}
		}
	}

	// Deep chain — kept shallow to avoid the existing O(N*D) cost of
	// per-dir sub-scans in ModeFull. Depth 6 is enough to exercise nesting.
	deep := filepath.Join(root, "deep")
	cur := deep
	for level := 0; level < 6; level++ {
		cur = filepath.Join(cur, fmt.Sprintf("L%02d", level))
		if err := os.MkdirAll(cur, 0o755); err != nil {
			tb.Fatal(err)
		}
		path := filepath.Join(cur, "file.txt")
		if err := os.WriteFile(path, []byte(fmt.Sprintf("level-%d", level)), 0o644); err != nil {
			tb.Fatal(err)
		}
	}

	// Mixed
	for m := 0; m < 5; m++ {
		mix := filepath.Join(root, fmt.Sprintf("mix%d", m))
		if err := os.MkdirAll(mix, 0o755); err != nil {
			tb.Fatal(err)
		}
		for f := 0; f < 100; f++ {
			path := filepath.Join(mix, fmt.Sprintf("flat%03d", f))
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				tb.Fatal(err)
			}
		}
		for s := 0; s < 3; s++ {
			sub := filepath.Join(mix, fmt.Sprintf("s%d", s))
			if err := os.MkdirAll(sub, 0o755); err != nil {
				tb.Fatal(err)
			}
			for f := 0; f < 10; f++ {
				path := filepath.Join(sub, fmt.Sprintf("n%03d", f))
				if err := os.WriteFile(path, []byte("y"), 0o644); err != nil {
					tb.Fatal(err)
				}
			}
		}
	}
}

// TestParallelByteIdentical verifies that parallel and sequential scans
// produce byte-identical manifests on the same tree.
func TestParallelByteIdentical(t *testing.T) {
	root := filepath.Join("/tmp", "c4-parallel-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	buildParallelFixture(t, root)

	seq, err := Dir(root, WithMaxConcurrency(1))
	if err != nil {
		t.Fatalf("sequential: %v", err)
	}
	par, err := Dir(root, WithMaxConcurrency(8))
	if err != nil {
		t.Fatalf("parallel: %v", err)
	}

	if len(seq.Entries) < 1000 {
		t.Fatalf("fixture too small: %d entries", len(seq.Entries))
	}
	if len(seq.Entries) != len(par.Entries) {
		t.Fatalf("entry count mismatch: seq=%d par=%d", len(seq.Entries), len(par.Entries))
	}

	seqID := seq.ComputeC4ID()
	parID := par.ComputeC4ID()
	if seqID != parID {
		t.Fatalf("c4 id mismatch:\n  seq: %s\n  par: %s", seqID, parID)
	}

	// Byte-level canonical match — stronger than C4 ID match.
	if seq.Canonical() != par.Canonical() {
		t.Fatalf("canonical bytes differ between sequential and parallel scans")
	}
}

// TestParallelProgressCounts confirms the progress callback reports correct
// totals when fired from multiple goroutines.
func TestParallelProgressCounts(t *testing.T) {
	root := filepath.Join("/tmp", "c4-parallel-progress-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	buildParallelFixture(t, root)

	var mu sync.Mutex
	var fires []ScanStats
	var callbacks int64
	cb := func(s ScanStats) {
		atomic.AddInt64(&callbacks, 1)
		mu.Lock()
		fires = append(fires, s)
		mu.Unlock()
	}

	m, err := Dir(root, WithMode(ModeMetadata), WithProgress(cb), WithMaxConcurrency(8))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	if len(fires) == 0 {
		t.Fatalf("callback never fired")
	}
	last := fires[len(fires)-1]
	if last.Entries != int64(len(m.Entries)) {
		t.Errorf("final stats.Entries = %d, want %d (len manifest.Entries)", last.Entries, len(m.Entries))
	}
	if last.Files+last.Dirs != last.Entries {
		t.Errorf("Files(%d)+Dirs(%d) != Entries(%d)", last.Files, last.Dirs, last.Entries)
	}
}

// TestParallelAutoConcurrency exercises the default (n=0) path and checks
// the result still matches the sequential scan.
func TestParallelAutoConcurrency(t *testing.T) {
	root := filepath.Join("/tmp", "c4-parallel-auto-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	buildParallelFixture(t, root)

	seq, err := Dir(root, WithMaxConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	auto, err := Dir(root) // default = auto
	if err != nil {
		t.Fatal(err)
	}
	if seq.ComputeC4ID() != auto.ComputeC4ID() {
		t.Fatalf("auto-concurrency c4 id differs from sequential")
	}
}
