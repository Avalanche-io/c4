package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// 1.0.27 must upgrade blind: `c4 id` in every scan mode writes what 1.0.26
// wrote. testdata/blind-upgrade/ holds 1.0.26's output (release/1.0.x
// ac75240) on three trees built with fixed modes and times: a flat one with
// an empty directory, a nested one with an empty directory two levels down,
// and one holding an unreadable directory (1.0.26 stops at it; so must this
// line, in every mode). Through the childless-directory size rule, a
// childless directory with no ID keeps a null size; a scan states 0 for a
// directory it listed as empty, so nothing here changes.
//
// To rewrite the goldens from a 1.0.26 binary:
//
//	C4_BLIND_UPGRADE_WRITE=/path/to/c4-1.0.26 go test ./cmd/c4 -run TestBlindUpgradeScanOutput
func TestBlindUpgradeScanOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes and the unreadable tree need Unix permissions")
	}
	bin := buildC4(t)
	writeWith := os.Getenv("C4_BLIND_UPGRADE_WRITE")
	if writeWith != "" {
		bin = writeWith
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": "", "HOME": home, "USERPROFILE": home}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	build := func(name string, files map[string]string, dirs []string) string {
		root := filepath.Join(dir, name)
		for _, d := range dirs {
			bmust(t, os.MkdirAll(filepath.Join(root, d), 0o755))
		}
		for rel, content := range files {
			p := filepath.Join(root, rel)
			bmust(t, os.MkdirAll(filepath.Dir(p), 0o755))
			bmust(t, os.WriteFile(p, []byte(content), 0o644))
		}
		// Fixed modes and times, deepest first so a directory's time is set
		// after its children are written.
		var paths []string
		bmust(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if p != root {
				paths = append(paths, p)
			}
			return nil
		}))
		sort.Slice(paths, func(i, j int) bool { return strings.Count(paths[i], "/") > strings.Count(paths[j], "/") })
		for _, p := range paths {
			info, err := os.Lstat(p)
			bmust(t, err)
			mode := os.FileMode(0o644)
			if info.IsDir() {
				mode = 0o755
			}
			bmust(t, os.Chmod(p, mode))
			bmust(t, os.Chtimes(p, when, when))
		}
		return root
	}

	trees := []struct {
		name  string
		root  string
		needs string
	}{
		{"flat", build("flat", map[string]string{"a.txt": "a\n"}, []string{"empty"}), ""},
		{"nested", build("nested", map[string]string{"z": "z\n", "a/f": "f\n"}, []string{"a/b/empty"}), ""},
		{"unread", build("unread", map[string]string{"f": "f\n", "locked/inner": "in\n"}, []string{"empty"}), "unread"},
	}
	if os.Geteuid() != 0 {
		locked := filepath.Join(dir, "unread", "locked")
		bmust(t, os.Chmod(locked, 0))
		t.Cleanup(func() { os.Chmod(locked, 0o755) })
	}

	golden := filepath.Join("testdata", "blind-upgrade")
	for _, tr := range trees {
		if tr.needs == "unread" && os.Geteuid() == 0 {
			continue
		}
		for _, mode := range []string{"s", "m", "f"} {
			out, _, code := runC4WithEnv(t, bin, env, "id", "-m", mode, tr.root)
			got := fmt.Sprintf("exit %d\n%s", code, out)
			file := filepath.Join(golden, tr.name+"-"+mode+".txt")
			if writeWith != "" {
				bmust(t, os.MkdirAll(golden, 0o755))
				bmust(t, os.WriteFile(file, []byte(got), 0o644))
				continue
			}
			want, err := os.ReadFile(file)
			bmust(t, err)
			if got != string(want) {
				t.Errorf("%s, -m %s: output differs from 1.0.26's\n--- 1.0.26\n%s--- this line\n%s", tr.name, mode, want, got)
			}
		}
	}
}

func bmust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
