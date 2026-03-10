package progscan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func TestPaddedLineFixedWidth(t *testing.T) {
	name := "example.txt"
	depth := 1

	// Phase 0: type-only mode, null everything else.
	line0 := PaddedLine(depth, os.ModeDir, time.Time{}, -1, name, c4.ID{})

	// Phase 1: full mode, real timestamp, real size, null C4 ID.
	ts := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	line1 := PaddedLine(depth, 0755|os.ModeDir, ts, 4521, name, c4.ID{})

	// Phase 2: everything filled.
	id := c4.Identify(strings.NewReader("test content"))
	line2 := PaddedLine(depth, 0755|os.ModeDir, ts, 4521, name, id)

	if len(line0) != len(line1) {
		t.Errorf("phase 0 (%d) != phase 1 (%d) line width", len(line0), len(line1))
	}
	if len(line1) != len(line2) {
		t.Errorf("phase 1 (%d) != phase 2 (%d) line width", len(line1), len(line2))
	}

	t.Logf("line width: %d bytes", len(line0))
	t.Logf("phase 0: %s", string(line0))
	t.Logf("phase 1: %s", string(line1))
	t.Logf("phase 2: %s", string(line2))
}

func TestPaddedLineParseable(t *testing.T) {
	ts := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	id := c4.Identify(strings.NewReader("hello"))

	tests := []struct {
		name  string
		depth int
		mode  os.FileMode
		ts    time.Time
		size  int64
		fname string
		id    c4.ID
	}{
		{"all null", 0, os.ModeDir, time.Time{}, -1, "dir/", c4.ID{}},
		{"metadata only", 0, 0644, ts, 1234, "file.txt", c4.ID{}},
		{"fully resolved", 1, 0644, ts, 1234, "file.txt", id},
		{"large size", 0, 0644, ts, 999999999999999, "big.dat", id},
		{"indented", 1, 0644, ts, 500, "nested.go", id},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := PaddedLine(tt.depth, tt.mode, tt.ts, tt.size, tt.fname, tt.id)

			// Parse through the standard c4m decoder.
			m, err := c4m.NewDecoder(bytes.NewReader(line)).Decode()
			if err != nil {
				t.Fatalf("c4m decode failed: %v\nline: %q", err, string(line))
			}
			if len(m.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Entries))
			}

			e := m.Entries[0]
			if e.Depth != tt.depth {
				t.Errorf("depth: got %d, want %d", e.Depth, tt.depth)
			}
			if e.Name != tt.fname {
				// c4m strips trailing / from dir names in the Name field
				wantName := strings.TrimSuffix(tt.fname, "/")
				if e.Name != wantName {
					t.Errorf("name: got %q, want %q", e.Name, wantName)
				}
			}
			if !tt.id.IsNil() && e.C4ID != tt.id {
				t.Errorf("c4id: got %s, want %s", e.C4ID, tt.id)
			}
		})
	}
}

func TestThreePhases(t *testing.T) {
	// Create a temp directory with known structure.
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Hello\n")
	writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	mkDir(t, root, "src")
	writeFile(t, root, "src/lib.go", "package src\n")
	writeFile(t, root, "src/util.go", "package src\n\nfunc Util() {}\n")
	mkDir(t, root, "src/internal")
	writeFile(t, root, "src/internal/helper.go", "package internal\n")
	mkDir(t, root, "docs")
	writeFile(t, root, "docs/guide.md", "# Guide\n")

	outPath := filepath.Join(t.TempDir(), "test.c4m")

	sc, err := New(root, outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()

	// --- Phase 0: Structure ---
	if err := sc.Phase0(); err != nil {
		t.Fatal("phase 0:", err)
	}
	t.Logf("Phase 0: %d files, %d dirs", sc.Files, sc.Dirs)

	phase0 := readFile(t, outPath)
	t.Logf("--- Phase 0 (structure) ---\n%s", phase0)

	// Verify parseable.
	m0 := mustParse(t, phase0, "phase 0")
	if len(m0.Entries) != 9 { // 4 files + 2 dirs + 2 files + 1 dir contents
		t.Errorf("phase 0: expected 9 entries, got %d", len(m0.Entries))
	}

	// All C4 IDs should be nil.
	for _, e := range m0.Entries {
		if !e.C4ID.IsNil() {
			t.Errorf("phase 0: entry %q has non-nil C4 ID", e.Name)
		}
	}

	// --- Phase 1: Metadata ---
	if err := sc.Phase1(); err != nil {
		t.Fatal("phase 1:", err)
	}

	phase1 := readFile(t, outPath)
	t.Logf("--- Phase 1 (metadata) ---\n%s", phase1)

	m1 := mustParse(t, phase1, "phase 1")
	for _, e := range m1.Entries {
		if e.C4ID.IsNil() == false {
			t.Errorf("phase 1: entry %q should not have C4 ID yet", e.Name)
		}
		// Files should have real sizes.
		if !e.IsDir() && e.Size < 0 {
			t.Errorf("phase 1: file %q still has null size", e.Name)
		}
	}

	// Verify line widths unchanged.
	if len(phase0) != len(phase1) {
		t.Errorf("file size changed: phase 0 = %d, phase 1 = %d", len(phase0), len(phase1))
	}

	// --- Phase 2: Identity ---
	if err := sc.Phase2(); err != nil {
		t.Fatal("phase 2:", err)
	}

	phase2 := readFile(t, outPath)
	t.Logf("--- Phase 2 (identity) ---\n%s", phase2)

	m2 := mustParse(t, phase2, "phase 2")
	for _, e := range m2.Entries {
		if e.IsDir() {
			continue
		}
		if e.C4ID.IsNil() {
			t.Errorf("phase 2: file %q still has null C4 ID", e.Name)
		}
	}

	// Verify line widths unchanged.
	if len(phase1) != len(phase2) {
		t.Errorf("file size changed: phase 1 = %d, phase 2 = %d", len(phase1), len(phase2))
	}

	// --- Compact ---
	var compact bytes.Buffer
	if err := sc.Compact(&compact); err != nil {
		t.Fatal("compact:", err)
	}
	t.Logf("--- Compacted ---\n%s", compact.String())
	t.Logf("Working file: %d bytes, Compacted: %d bytes (%.0f%% reduction)",
		len(phase2), compact.Len(),
		100*(1-float64(compact.Len())/float64(len(phase2))))

	// Verify compacted is also parseable with same entries.
	m3 := mustParse(t, compact.Bytes(), "compact")
	if len(m3.Entries) != len(m2.Entries) {
		t.Errorf("compact: %d entries, want %d", len(m3.Entries), len(m2.Entries))
	}
}

// TestRealDirectory scans a real directory when SCAN_DIR is set.
// Run with: SCAN_DIR=~/ws go test -run TestRealDirectory -v -timeout 600s
func TestRealDirectory(t *testing.T) {
	dir := os.Getenv("SCAN_DIR")
	if dir == "" {
		t.Skip("set SCAN_DIR to run real filesystem scan")
	}

	outPath := filepath.Join(t.TempDir(), "scan.c4m")

	sc, err := New(dir, outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()

	// Phase 0.
	start := time.Now()
	if err := sc.Phase0(); err != nil {
		t.Fatal("phase 0:", err)
	}
	fi0, _ := os.Stat(outPath)
	t.Logf("Phase 0: %d files, %d dirs in %s (%.1f MB)",
		sc.Files, sc.Dirs, time.Since(start).Round(time.Millisecond),
		float64(fi0.Size())/(1024*1024))

	// Show first 20 lines.
	showHead(t, outPath, 20, "Phase 0")

	// Phase 1.
	start = time.Now()
	if err := sc.Phase1(); err != nil {
		t.Fatal("phase 1:", err)
	}
	fi1, _ := os.Stat(outPath)
	t.Logf("Phase 1: metadata in %s (%.1f MB, delta=%d bytes)",
		time.Since(start).Round(time.Millisecond),
		float64(fi1.Size())/(1024*1024),
		fi1.Size()-fi0.Size())

	showHead(t, outPath, 20, "Phase 1")

	// Phase 2 (only if small enough to be practical in a test).
	if sc.Files <= 10000 {
		start = time.Now()
		if err := sc.Phase2(); err != nil {
			t.Fatal("phase 2:", err)
		}
		fi2, _ := os.Stat(outPath)
		t.Logf("Phase 2: C4 IDs in %s (%.1f MB, delta=%d bytes)",
			time.Since(start).Round(time.Millisecond),
			float64(fi2.Size())/(1024*1024),
			fi2.Size()-fi1.Size())

		showHead(t, outPath, 20, "Phase 2")
	} else {
		t.Logf("Phase 2: skipped (%d files too many for test)", sc.Files)
	}
}

// helpers

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mkDir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel), 0755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustParse(t *testing.T, data []byte, label string) *c4m.Manifest {
	t.Helper()
	m, err := c4m.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		t.Fatalf("%s: c4m parse failed: %v\nfirst 500 bytes: %s", label, err, string(data[:min(500, len(data))]))
	}
	return m
}

func showHead(t *testing.T, path string, n int, label string) {
	t.Helper()
	data, _ := os.ReadFile(path)
	lines := strings.SplitN(string(data), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	t.Logf("--- %s (first %d lines) ---", label, len(lines))
	for _, l := range lines {
		t.Logf("  %s", l)
	}
	if len(data) > 0 {
		total := strings.Count(string(data), "\n")
		t.Logf("  ... (%d total lines)", total)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
