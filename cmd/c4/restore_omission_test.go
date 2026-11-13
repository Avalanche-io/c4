package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// A description holding a file with no C4 ID (name, size and date known,
// content not) restores everything else, keeps that file as found, names
// it by its full path, verifies what it wrote, and exits 2. Writing zeros
// of the stated size would invent content (decided 2026-11-13 by the
// release lead; reversible). Through 2026-11-13 verification compared the
// omitted file too, so restore failed, exit 3, with a "re-run the restore"
// hint no re-run could satisfy.
func TestRestoreKeepsAnEntryWithNoIDAndVerifiesTheRest(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}
	if err := os.MkdirAll(env["C4_STORE"], 0o755); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"a.txt": "alpha", "d/x.txt": "x", "d/y.txt": "y"})
	if _, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", src); code != 0 {
		t.Fatalf("storing src: exit %d: %s", code, stderr)
	}
	described, stderr, code := runC4WithEnv(t, bin, env, "id", src)
	if code != 0 {
		t.Fatalf("c4 id src: exit %d: %s", code, stderr)
	}
	// d/x.txt's content is unknown: clear its C4 ID through the API.
	m, err := c4m.DecodeResolved(strings.NewReader(described), nil)
	if err != nil {
		t.Fatal(err)
	}
	cleared := false
	for p, e := range c4m.EntryPaths(m.Entries) {
		if p == "d/x.txt" {
			e.C4ID = c4.ID{}
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("d/x.txt not in the description")
	}
	c4m.Normalize(m)
	text, err := c4m.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	desc := filepath.Join(dir, "partial.c4m")
	if err := os.WriteFile(desc, text, 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := runC4WithEnv(t, bin, env, "restore", "--force", desc, dst)
	if code != 2 {
		t.Fatalf("restore with an omission must exit 2, got %d:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "incomplete entry kept as found: d/x.txt") {
		t.Errorf("the omission must be named by its full path:\n%s", stderr)
	}
	if strings.Contains(stderr, "re-run") || strings.Contains(stderr, "verification failed") {
		t.Errorf("an omission is not a curable failure:\n%s", stderr)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 2 {
		t.Errorf("stdout must be the pre-image and as-built IDs, got:\n%s", out)
	}
	for rel, want := range map[string]string{"a.txt": "alpha", "d/y.txt": "y"} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil || string(got) != want {
			t.Errorf("%s: got %q, %v; want %q", rel, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "d", "x.txt")); !os.IsNotExist(err) {
		t.Errorf("d/x.txt has no content to write and must not be created: %v", err)
	}
}
