package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// backdateTree sets every file and directory under root to old, deepest
// first, so the tree's recorded times can never collide with "now".
func backdateTree(t *testing.T, root string, old time.Time) {
	t.Helper()
	var paths []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && p != root {
			paths = append(paths, p)
		}
		return nil
	})
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], string(filepath.Separator)) >
			strings.Count(paths[j], string(filepath.Separator))
	})
	for _, p := range paths {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPatchSyncRestoresNestedDirTimes is the mac/vm round trip: a tree
// is synced to a second location, edited there the way git edits a
// repository (appends in place, ref rewrites by rename, new object dirs,
// deletions three levels down), then synced back. Directories whose
// time did not change on the far side still have every file beneath
// them rewritten by the patch; afterwards "c4 id" of the patched tree
// must equal the far side's listing exactly, directory times included.
func TestPatchSyncRestoresNestedDirTimes(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}
	mac := filepath.Join(dir, "mac")
	vm := filepath.Join(dir, "vm")

	writeTree(t, mac, map[string]string{
		"a.txt":                  "a\n",
		"g/HEAD":                 "ref: refs/heads/main\n",
		"g/logs/HEAD":            "one\n",
		"g/logs/refs/heads/main": "one\n",
		"g/refs/heads/main":      "1111\n",
		"g/objects/aa/x":         "obj x",
		"g/d1/d2/d3/keep.txt":    "keep v1",
		"g/d1/d2/d3/old.txt":     "old",
	})
	backdateTree(t, mac, time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	base := filepath.Join(dir, "base.c4m")
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", mac)
	if code != 0 {
		t.Fatalf("id mac exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(base, []byte(out), 0644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code = runC4WithEnv(t, bin, env, "patch", base, vm+"/"); code != 0 {
		t.Fatalf("patch base vm exit %d: %s", code, stderr)
	}

	// Edit vm. In-place writes leave their directory's time alone, so
	// those directories keep the times they share with mac.
	appendTo := func(rel, s string) {
		f, err := os.OpenFile(filepath.Join(vm, filepath.FromSlash(rel)), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	appendTo("g/logs/HEAD", "two\n")
	appendTo("g/logs/refs/heads/main", "two\n")
	appendTo("g/d1/d2/d3/keep.txt", " v2")
	appendTo("a.txt", "b\n")
	lock := filepath.Join(vm, "g", "refs", "heads", "main.lock")
	if err := os.WriteFile(lock, []byte("2222\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(lock, filepath.Join(vm, "g", "refs", "heads", "main")); err != nil {
		t.Fatal(err)
	}
	writeTree(t, vm, map[string]string{"g/objects/bb/y": "obj y"})
	if err := os.Remove(filepath.Join(vm, "g", "d1", "d2", "d3", "old.txt")); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "vm.c4m")
	want, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", vm)
	if code != 0 {
		t.Fatalf("id vm exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(target, []byte(want), 0644); err != nil {
		t.Fatal(err)
	}

	if _, stderr, code = runC4WithEnv(t, bin, env, "patch", target, mac); code != 0 {
		t.Fatalf("patch vm.c4m mac exit %d: %s", code, stderr)
	}
	got, stderr, code := runC4WithEnv(t, bin, env, "id", mac)
	if code != 0 {
		t.Fatalf("id mac exit %d: %s", code, stderr)
	}
	if got != want {
		wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
		for i := 0; i < len(wl) && i < len(gl); i++ {
			if wl[i] != gl[i] {
				t.Errorf("line %d:\n  want %s\n  got  %s", i+1, wl[i], gl[i])
			}
		}
		t.Fatalf("patched tree does not reproduce the target listing")
	}
}
