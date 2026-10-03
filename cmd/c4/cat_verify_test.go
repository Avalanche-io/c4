package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/store"
)

// putObject stores data in a TreeStore at root and returns its ID and
// the path of the object file.
func putObject(t *testing.T, root string, data string) (string, string) {
	t.Helper()
	s, err := store.NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Put(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var objPath string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == id.String() {
			objPath = p
		}
		return nil
	})
	if objPath == "" {
		t.Fatalf("object %s not found under %s", id, root)
	}
	return id.String(), objPath
}

// tamper overwrites a store object in place with different bytes.
func tamper(t *testing.T, path, data string) {
	t.Helper()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCatIntactObject(t *testing.T) {
	bin := buildC4(t)
	root := filepath.Join(t.TempDir(), "store")
	const data = "original bytes\n"
	id, _ := putObject(t, root, data)

	env := map[string]string{"C4_STORE": root}
	out, stderr, code := runC4WithEnv(t, bin, env, "cat", id)
	if code != 0 {
		t.Fatalf("cat exit %d: %s", code, stderr)
	}
	if out != data {
		t.Errorf("cat output %q, want %q", out, data)
	}
}

func TestCatTamperedObjectFails(t *testing.T) {
	bin := buildC4(t)
	root := filepath.Join(t.TempDir(), "store")
	id, objPath := putObject(t, root, "original bytes\n")
	tamper(t, objPath, "tampered bytes\n")

	env := map[string]string{"C4_STORE": root}
	out, stderr, code := runC4WithEnv(t, bin, env, "cat", id)
	if code != 1 {
		t.Errorf("cat of tampered object exit %d, want 1", code)
	}
	if out != "" {
		t.Errorf("cat emitted unverified bytes: %q", out)
	}
	if !strings.Contains(stderr, id) || !strings.Contains(stderr, "does not match") {
		t.Errorf("stderr should name the ID and the mismatch, got: %q", stderr)
	}
}

func TestCatErgonomicTamperedC4mFails(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	listing, _, code := runC4(t, bin, "id", tree)
	if code != 0 || !strings.Contains(listing, "a.txt") {
		t.Fatalf("id exit %d: %q", code, listing)
	}

	root := filepath.Join(dir, "store")
	id, objPath := putObject(t, root, listing)
	env := map[string]string{"C4_STORE": root}

	// Intact: -e succeeds.
	if _, stderr, code := runC4WithEnv(t, bin, env, "cat", "-e", id); code != 0 {
		t.Fatalf("cat -e on intact c4m exit %d: %s", code, stderr)
	}

	// Tampered but still valid c4m: must not be parsed and printed.
	tamper(t, objPath, strings.Replace(listing, "a.txt", "b.txt", 1))
	for _, flag := range []string{"-e", "-r"} {
		out, stderr, code := runC4WithEnv(t, bin, env, "cat", flag, id)
		if code != 1 {
			t.Errorf("cat %s on tampered c4m exit %d, want 1", flag, code)
		}
		if out != "" {
			t.Errorf("cat %s emitted unverified content: %q", flag, out)
		}
		if !strings.Contains(stderr, id) {
			t.Errorf("cat %s stderr should name the ID, got: %q", flag, stderr)
		}
	}
}
