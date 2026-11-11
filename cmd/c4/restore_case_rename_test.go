package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// A description that changes only a file's case is a rename, never a
// removal. On a folder that folds case, `Readme.md` on disk is the target's
// `readme.md` by identity; restore used to write the target's bytes into it
// and then remove it as unlisted, leaving the folder empty and failing its
// own verification (exit 3). Now the entry is kept, holds the target's
// bytes, is listed under exactly `readme.md`, and restore verifies (exit 0).
func TestRestoreCaseOnlyRenameKeepsTheFile(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "Readme.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := os.Lstat(filepath.Join(dest, "Readme.md"))
	if b, err := os.Lstat(filepath.Join(dest, "readme.md")); err != nil || !os.SameFile(a, b) {
		t.Skip("this volume tells Readme.md and readme.md apart")
	}

	bin := buildC4(t)
	storeDir := filepath.Join(work, "store")
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Put(strings.NewReader("new\n"))
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "readme.md", Mode: 0o644, Timestamp: ts, Size: 4, C4ID: id})
	c4m.Normalize(target)
	var buf bytes.Buffer
	if err := c4m.NewEncoder(&buf).Encode(target); err != nil {
		t.Fatal(err)
	}
	desc := filepath.Join(work, "target.c4m")
	if err := os.WriteFile(desc, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": storeDir}
	out, errOut, code := runC4WithEnv(t, bin, env, "restore", "--force", desc, dest)
	if code != 0 {
		t.Fatalf("restore exited %d, want 0\nstdout: %s\nstderr: %.600s", code, out, errOut)
	}
	names, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range names {
		got = append(got, n.Name())
	}
	if len(got) != 1 || got[0] != "readme.md" {
		t.Fatalf("folder lists %q, want exactly readme.md", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "readme.md")); string(b) != "new\n" {
		t.Errorf("readme.md = %q, want the target's bytes", b)
	}
}
