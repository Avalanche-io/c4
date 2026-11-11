package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A store object whose bytes no longer hash to its ID (tampered, or
// damaged on disk) must never be written over a file. Master's restore
// copied it into place and only then failed its verification (exit 3),
// leaving "evil content" in the user's a.txt. Release 1.0.x has refused it
// since 5b05207; master never took it. The bytes are now hashed as they
// are copied and published only when they match, so a.txt keeps its
// original content.
func TestRestoreRefusesATamperedStoreObject(t *testing.T) {
	bin := buildC4(t)
	work := t.TempDir()
	tree := filepath.Join(work, "tree")
	dst := filepath.Join(work, "dst")
	storeDir := filepath.Join(work, "store")
	for _, d := range []string{tree, dst} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("good content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "b.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": filepath.Join(work, "home"), "USERPROFILE": filepath.Join(work, "home"), "C4_STORE": storeDir}
	out, errOut, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("c4 id -s exited %d: %s", code, errOut)
	}
	snap := strings.TrimSpace(out)
	idOut, _, _ := runC4WithEnv(t, bin, env, "id", "-q", filepath.Join(tree, "a.txt"))
	aID := strings.TrimSpace(idOut)

	// Damage a.txt's object in the store.
	var obj string
	filepath.Walk(storeDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Base(p) == aID {
			obj = p
		}
		return nil
	})
	if obj == "" {
		t.Fatalf("a.txt's object %s not found in the store", aID)
	}
	os.Chmod(obj, 0o644)
	if err := os.WriteFile(obj, []byte("evil content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "a.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errOut, code = runC4WithEnv(t, bin, env, "restore", "--force", snap, dst)
	if code == 0 {
		t.Errorf("restore from a tampered store exited 0")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "a.txt")); string(b) != "original\n" {
		t.Fatalf("a.txt = %q after restore: unverified store bytes replaced the file (stderr: %.300s)", b, errOut)
	}
	if !strings.Contains(errOut, "does not match its ID") {
		t.Errorf("stderr does not say the content failed its ID: %.400s", errOut)
	}
}
