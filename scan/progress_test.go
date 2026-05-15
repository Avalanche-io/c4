package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// buildProgressFixture creates a tree with at least 2000 entries under root.
// Layout: 20 directories, each with 100 small files = 2020 entries.
func buildProgressFixture(t testing.TB, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	for d := 0; d < 20; d++ {
		dir := filepath.Join(root, fmt.Sprintf("d%02d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		for f := 0; f < 100; f++ {
			path := filepath.Join(dir, fmt.Sprintf("f%03d.bin", f))
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}
	}
}

func TestWithProgress(t *testing.T) {
	root := filepath.Join("/tmp", "c4-progress-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	buildProgressFixture(t, root)

	var mu sync.Mutex
	var fires []ScanStats
	cb := func(s ScanStats) {
		mu.Lock()
		fires = append(fires, s)
		mu.Unlock()
	}

	// Use ModeMetadata to skip C4 ID computation — faster, plus avoids
	// the per-dir sub-scan, so the entry counts stay easy to reason about.
	m, err := Dir(root, WithMode(ModeMetadata), WithProgress(cb))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	if len(fires) < 2 {
		t.Fatalf("callback fired %d times, want at least 2", len(fires))
	}

	last := fires[len(fires)-1]
	if last.Entries != int64(len(m.Entries)) {
		t.Errorf("final stats.Entries = %d, want %d (len manifest.Entries)", last.Entries, len(m.Entries))
	}
	if last.Files+last.Dirs != last.Entries {
		t.Errorf("Files(%d)+Dirs(%d) != Entries(%d)", last.Files, last.Dirs, last.Entries)
	}
	if last.Elapsed <= 0 {
		t.Errorf("Elapsed = %v, want > 0", last.Elapsed)
	}
}

func TestWithProgressNilSafe(t *testing.T) {
	root := filepath.Join("/tmp", "c4-progress-nil-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Passing nil should be a no-op, not panic.
	if _, err := Dir(root, WithProgress(nil)); err != nil {
		t.Fatalf("Dir with nil progress: %v", err)
	}
}

// BenchmarkScanNoProgress and BenchmarkScanWithProgress confirm zero overhead
// when no callback is registered. Run with: go test -bench=Scan -benchmem ./scan/
func benchFixture(b *testing.B, root string) {
	b.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		b.Fatalf("mkdir: %v", err)
	}
	for d := 0; d < 5; d++ {
		dir := filepath.Join(root, fmt.Sprintf("d%d", d))
		_ = os.MkdirAll(dir, 0o755)
		for f := 0; f < 40; f++ {
			_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d", f)), []byte("x"), 0o644)
		}
	}
}

func BenchmarkScanNoProgress(b *testing.B) {
	root := filepath.Join(b.TempDir(), "tree")
	benchFixture(b, root)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Dir(root, WithMode(ModeMetadata)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanWithProgress(b *testing.B) {
	root := filepath.Join(b.TempDir(), "tree")
	benchFixture(b, root)
	cb := func(ScanStats) {}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Dir(root, WithMode(ModeMetadata), WithProgress(cb)); err != nil {
			b.Fatal(err)
		}
	}
}
