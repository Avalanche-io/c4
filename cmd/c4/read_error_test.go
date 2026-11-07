package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A path that opens but fails to read (a directory opened as a file
// gives EISDIR on read, the same shape as a non-local file on a mount
// giving EIO) yields no ID and the read error, never the ID of the
// bytes read so far or of empty input. (Storage lead, 2026-11-07.)
func TestIdentifyFileBytesReportsAReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opening a directory as a file fails at open on Windows")
	}
	dir := t.TempDir()
	id, err := identifyFileBytes(dir)
	if err == nil {
		t.Fatalf("a failed read returned no error (id %s)", id)
	}
	if !id.IsNil() {
		t.Fatalf("a failed read returned an ID: %s", id)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("the error does not name the path: %v", err)
	}
}

// `c4 id FILE` on a file that cannot be read says so and exits 1, in
// -q and without it; it does not blame the level, and prints no entry.
func TestIDSaysCannotReadWhenTheReadFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 does not restrict reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.txt")
	if err := os.WriteFile(locked, []byte("unreadable"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o644)

	for _, args := range [][]string{{"id", locked}, {"id", "-q", locked}} {
		stdout, stderr, code := runC4(t, bin, args...)
		if code != 1 {
			t.Fatalf("%v: exit %d, want 1\n%s", args, code, stderr)
		}
		if !strings.Contains(stderr, "cannot read") || strings.Contains(stderr, "does not hash") {
			t.Fatalf("%v: stderr should say the read failed, not blame the level:\n%s", args, stderr)
		}
		if strings.TrimSpace(stdout) != "" {
			t.Fatalf("%v: an unreadable file printed something:\n%s", args, stdout)
		}
	}
}
