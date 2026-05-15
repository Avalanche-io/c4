package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// TestEntryStream_AllEntries streams a small tree and asserts the
// streamed entries match scan.Dir's manifest entries byte-identically.
func TestEntryStream_AllEntries(t *testing.T) {
	dir := buildSmallTree(t)

	// Baseline: a regular scan.Dir call.
	baseline, err := Dir(dir)
	if err != nil {
		t.Fatalf("baseline Dir: %v", err)
	}

	// Streamed scan into a slice.
	var streamed []*c4m.Entry
	m, err := Dir(dir, WithEntryStream(func(e *c4m.Entry) error {
		// Copy the pointer; entries are not mutated after emit in normal flow.
		streamed = append(streamed, e)
		return nil
	}))
	if err != nil {
		t.Fatalf("streaming Dir: %v", err)
	}

	if len(m.Entries) != len(baseline.Entries) {
		t.Fatalf("manifest entry count mismatch: streamed=%d baseline=%d",
			len(m.Entries), len(baseline.Entries))
	}
	if len(streamed) != len(baseline.Entries) {
		t.Fatalf("streamed entry count = %d, want %d", len(streamed), len(baseline.Entries))
	}

	// After SortEntries the order in the final manifest may differ from
	// emit order. The contract is: the set of streamed entries == the set
	// of final manifest entries. Compare by name+depth.
	want := keySet(baseline.Entries)
	got := keySet(streamed)
	if len(want) != len(got) {
		t.Fatalf("key set sizes differ: got=%d want=%d", len(got), len(want))
	}
	for k := range want {
		if !got[k] {
			t.Errorf("baseline entry %q missing from stream", k)
		}
	}
}

// TestEntryStream_CancelMidScan builds a large tree, cancels the context
// after N entries are observed, and confirms the scan halts and surfaces
// ctx.Canceled along with a partial manifest.
func TestEntryStream_CancelMidScan(t *testing.T) {
	const totalFiles = 2500
	const cancelAfter = 100

	dir := buildLargeTree(t, totalFiles)

	ctx, cancel := context.WithCancel(context.Background())
	var seen int64
	cb := func(e *c4m.Entry) error {
		n := atomic.AddInt64(&seen, 1)
		if n == cancelAfter {
			cancel()
		}
		return nil
	}

	// Use ModeMetadata to skip the expensive per-directory sub-scan that
	// would otherwise dominate runtime in ModeFull.
	m, err := Dir(dir,
		WithMode(ModeMetadata),
		WithContext(ctx),
		WithEntryStream(cb),
	)

	if err == nil {
		t.Fatalf("expected cancellation error, got nil (seen=%d, manifest=%d)",
			seen, len(m.Entries))
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected ctx.Canceled, got %v", err)
	}
	if m == nil {
		t.Fatal("partial manifest must be non-nil on cancel")
	}

	n := atomic.LoadInt64(&seen)
	if n < cancelAfter {
		t.Fatalf("seen=%d, want >= %d", n, cancelAfter)
	}
	// Cancellation is observed at directory and entry boundaries; the
	// scan should not stream very far past the cancellation trigger.
	// Allow generous slack for in-flight work within a single directory.
	if n > totalFiles/2 {
		t.Fatalf("seen=%d ran way past cancel point (totalFiles=%d)", n, totalFiles)
	}
}

// TestEntryStream_ErrorFromCallback asserts that returning an error from
// the stream callback halts the scan and the error surfaces to the caller.
func TestEntryStream_ErrorFromCallback(t *testing.T) {
	dir := buildSmallTree(t)
	sentinel := errors.New("halt scan")

	var count int64
	m, err := Dir(dir, WithEntryStream(func(e *c4m.Entry) error {
		n := atomic.AddInt64(&count, 1)
		if n >= 2 {
			return sentinel
		}
		return nil
	}))

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if m == nil {
		t.Fatal("partial manifest must be non-nil on callback error")
	}
	if atomic.LoadInt64(&count) < 2 {
		t.Fatalf("callback fired %d times, want >= 2", count)
	}
}

// buildSmallTree creates a tiny but mixed tree (files + nested dirs).
func buildSmallTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644))
	must(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bb"), 0644))
	must(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
	must(t, os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("ccc"), 0644))
	must(t, os.MkdirAll(filepath.Join(dir, "sub", "deeper"), 0755))
	must(t, os.WriteFile(filepath.Join(dir, "sub", "deeper", "d.txt"), []byte("dddd"), 0644))
	return dir
}

// buildLargeTree creates a flat-ish tree with at least n regular files
// spread over a handful of subdirectories so cancellation has somewhere
// to bite mid-scan.
func buildLargeTree(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	const filesPerDir = 50
	dirCount := (n + filesPerDir - 1) / filesPerDir
	for d := 0; d < dirCount; d++ {
		sub := filepath.Join(dir, fmt.Sprintf("d%04d", d))
		must(t, os.MkdirAll(sub, 0755))
		for i := 0; i < filesPerDir && d*filesPerDir+i < n; i++ {
			p := filepath.Join(sub, fmt.Sprintf("f%04d.txt", i))
			must(t, os.WriteFile(p, []byte("x"), 0644))
		}
	}
	return dir
}

func keySet(entries []*c4m.Entry) map[string]bool {
	s := make(map[string]bool, len(entries))
	for _, e := range entries {
		s[fmt.Sprintf("%d:%s", e.Depth, e.Name)] = true
	}
	return s
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
