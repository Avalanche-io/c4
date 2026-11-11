package main

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

// Every verb that reads a c4m whose entry skips past its parent directory
// exits 1 naming the entry. On master they exited 2 with
// "panic: runtime error: index out of range" (release 1.0.x fixed this in
// 59ce7cc and 6cfba2c; master never took it). The damaged file is written
// by the library: an ordinary base, then a patch holding y.txt at depth 1
// under a file.
func TestVerbsRefuseADepthJumpWithoutPanicking(t *testing.T) {
	bin := buildC4(t)
	work := t.TempDir()
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	file := func(name string, depth int) *c4m.Entry {
		return &c4m.Entry{Name: name, Depth: depth, Mode: 0o644, Timestamp: ts, Size: 1,
			C4ID: c4.Identify(strings.NewReader(name))}
	}
	base := c4m.NewManifest()
	base.AddEntry(file("a.txt", 0))
	patch := c4m.NewManifest()
	patch.AddEntry(file("z.txt", 0))
	patch.AddEntry(file("y.txt", 1))
	var b bytes.Buffer
	if err := c4m.NewEncoder(&b).Encode(base); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(work, "good.c4m")
	if err := os.WriteFile(good, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c4m.NewEncoder(&b).EncodePatch(&c4m.PatchResult{OldID: base.ComputeC4ID(), Patch: patch}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(work, "bad.c4m")
	if err := os.WriteFile(bad, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(work, "dest")
	os.MkdirAll(dest, 0o755)

	for _, args := range [][]string{
		{"patch", bad},
		{"restore", "--force", bad, dest},
		{"log", bad},
		{"diff", bad, good},
		{"diff", good, bad},
		{"split", bad, "2", filepath.Join(work, "b.c4m"), filepath.Join(work, "a.c4m")},
		{"merge", bad, good},
		{"paths", bad},
		{"id", bad},
	} {
		_, errOut, code := runC4(t, bin, args...)
		if strings.Contains(errOut, "panic") || strings.Contains(errOut, "goroutine") {
			t.Errorf("c4 %s panicked: %.300s", args[0], errOut)
			continue
		}
		if code != 1 || !strings.Contains(errOut, `"y.txt" at depth 1 has no parent directory`) {
			t.Errorf("c4 %s: exit %d, stderr %.300s; want exit 1 naming y.txt", strings.Join(args[:1], " "), code, errOut)
		}
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("restore wrote into dest: %v", entries)
	}
}
