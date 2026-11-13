package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// `c4 cat -r --content` names every file whose content the store does not
// hold and exits 2; with everything present it exits 0. Before, `cat -r`
// checked listings only, so a snapshot naming content that was never
// stored passed with exit 0 (decided 2026-11-13 by the release lead;
// reversible).
func TestCatRecursiveContentNamesMissingContent(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir, "HOME": home, "USERPROFILE": home}

	tree := filepath.Join(dir, "t")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "sub/b.txt": "bravo"})
	snap, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("storing: exit %d: %s", code, stderr)
	}
	snap = strings.TrimSpace(snap)

	// Everything present: exit 0, nothing named.
	if _, stderr, code := runC4WithEnv(t, bin, env, "cat", "-r", "--content", snap); code != 0 || strings.Contains(stderr, "partial:") {
		t.Fatalf("all present: exit %d: %s", code, stderr)
	}

	// --content without -r is refused.
	if _, _, code := runC4WithEnv(t, bin, env, "cat", "--content", snap); code != 1 {
		t.Errorf("--content without -r must exit 1, got %d", code)
	}

	// Remove sub/b.txt's content from this test's own store.
	bID := c4.Identify(strings.NewReader("bravo")).String()
	removed := false
	filepath.Walk(storeDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Base(p) == bID {
			removed = os.Remove(p) == nil
		}
		return nil
	})
	if !removed {
		t.Fatal("could not find b.txt's object in the test store")
	}

	out, stderr, code := runC4WithEnv(t, bin, env, "cat", "-r", "--content", snap)
	if code != 2 {
		t.Fatalf("missing content must exit 2, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "partial: sub/b.txt content "+bID+" not in store") {
		t.Errorf("the missing file must be named by path and ID:\n%s", stderr)
	}
	if strings.Contains(stderr, "a.txt") {
		t.Errorf("a present file must not be named:\n%s", stderr)
	}
	if !strings.Contains(out, "b.txt") {
		t.Errorf("stdout is still the expanded tree:\n%s", out)
	}

	// Plain -r checks listings only, as before: exit 0.
	if _, _, code := runC4WithEnv(t, bin, env, "cat", "-r", snap); code != 0 {
		t.Errorf("cat -r without --content checks listings only, want exit 0, got %d", code)
	}

	// A c4m file on disk naming content the store never received.
	writeTree(t, tree, map[string]string{"c.txt": "charlie"})
	desc, stderr, code := runC4WithEnv(t, bin, env, "id", tree)
	if code != 0 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	descPath := filepath.Join(dir, "t.c4m")
	if err := os.WriteFile(descPath, []byte(desc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runC4WithEnv(t, bin, env, "cat", "-r", "--content", descPath)
	if code != 2 || !strings.Contains(stderr, "partial: c.txt content") {
		t.Errorf("a description naming unstored content: exit %d, stderr %s", code, stderr)
	}
}
