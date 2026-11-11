package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// A name this platform cannot hold is refused, never renamed, mapped or
// split into a path: `c4 restore --force` writes the rest, names each
// refusal on stderr, verifies the rest exactly, and exits 2. On Windows
// `a\b.txt` used to become `a` then `b.txt` through filepath.FromSlash;
// on darwin a name that is not UTF-8 cannot be created on APFS. Linux
// holds every name a c4m can carry, so there is nothing to refuse there.
func TestRestoreRefusesNamesThePlatformCannotHold(t *testing.T) {
	var bad string
	switch runtime.GOOS {
	case "windows":
		bad = `a\b.txt`
	case "darwin":
		bad = "bad\xff.txt"
	default:
		t.Skip("this platform holds every name a c4m can carry")
	}
	bin := buildC4(t)
	work := t.TempDir()
	storeDir := filepath.Join(work, "store")
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Modes this platform holds, so the rest verifies exactly: Windows
	// reads a writable file back as 0666 and a directory as 0777.
	fileMode, dirMode := os.FileMode(0o644), os.ModeDir|0o755
	if runtime.GOOS == "windows" {
		fileMode, dirMode = 0o666, os.ModeDir|0o777
	}
	file := func(name, content string, depth int) *c4m.Entry {
		id, err := s.Put(strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		return &c4m.Entry{Name: name, Depth: depth, Mode: fileMode, Timestamp: ts, Size: int64(len(content)), C4ID: id}
	}
	target := c4m.NewManifest()
	target.AddEntry(file("ok.txt", "ok\n", 0))
	target.AddEntry(file(bad, "bad\n", 0))
	target.AddEntry(&c4m.Entry{Name: "sub/", Mode: dirMode, Timestamp: ts, Size: -1})
	target.AddEntry(file("kept.txt", "kept\n", 1))
	target.SortEntries()
	c4m.Normalize(target)
	var b bytes.Buffer
	if err := c4m.NewEncoder(&b).Encode(target); err != nil {
		t.Fatal(err)
	}
	desc := filepath.Join(work, "target.c4m")
	if err := os.WriteFile(desc, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(work, "dest")
	env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": storeDir}
	out, errOut, code := runC4WithEnv(t, bin, env, "restore", "--force", desc, dest)
	if code != 2 {
		t.Fatalf("restore exited %d, want 2 (partial)\nstdout: %s\nstderr: %.600s", code, out, errOut)
	}
	if !strings.Contains(errOut, "refused: ") || !strings.Contains(errOut, "1 entries refused") {
		t.Errorf("stderr does not name the refusal and its count:\n%s", errOut)
	}
	if strings.Contains(errOut, "verification failed") {
		t.Errorf("the rest did not verify:\n%s", errOut)
	}
	// Line 2 of stdout is the verified as-built ID.
	if lines := strings.Fields(strings.TrimSpace(out)); len(lines) < 2 {
		t.Errorf("no verified as-built ID on stdout:\n%s", out)
	}
	for rel, want := range map[string]string{"ok.txt": "ok\n", "sub/kept.txt": "kept\n"} {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q (%v), want %q", rel, got, err, want)
		}
	}
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(filepath.Join(dest, "a")); err == nil {
			t.Errorf(`a\b.txt was split into a directory "a"`)
		}
	}
}
