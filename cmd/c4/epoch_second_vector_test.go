package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestEpochSecondVectorCLI runs the epoch-second vector
// (c4m/testdata/epoch-second-vector) through the CLI. Through 2026-11-16 a
// file timed inside the epoch second was written 1970-01-01T00:00:00Z,
// which reads back as null, so `c4 id dir/`'s boundary named a state its
// own reader could not reach ("patch ID does not match prior content"),
// and a directory's description ID differed from its c4m's.
func TestEpochSecondVectorCLI(t *testing.T) {
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "epoch-second-vector")
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}
	tree := filepath.Join(dir, "tree")
	buildEpochTree(t, filepath.Join(vec, "tree.txt"), tree)

	// run gives stdin to the command when it is not empty.
	run := func(stdin string, args ...string) string {
		t.Helper()
		out, stderr, code := runC4WithStdinEnv(t, bin, env, stdin, args...)
		if code != 0 {
			t.Fatalf("c4 %v: exit %d: %s", args, code, stderr)
		}
		return out
	}

	// c4's own scan output, written to a file and read every way back.
	scanText := run("", "id", tree+string(filepath.Separator))
	scanFile := filepath.Join(dir, "scan.c4m")
	if err := os.WriteFile(scanFile, []byte(scanText), 0o644); err != nil {
		t.Fatal(err)
	}
	flat := run("", "patch", scanFile)
	if fromStdin := run(scanText, "patch", "-"); fromStdin != flat {
		t.Errorf("c4 patch - writes\n%s\nbut c4 patch scan.c4m writes\n%s", fromStdin, flat)
	}
	for _, line := range strings.Split(strings.TrimSpace(flat), "\n") {
		f := strings.Fields(line)
		if (f[3] == "half.txt" || f[3] == "in.txt") && f[1] != "-" {
			t.Errorf("%s is written %q, want -", f[3], f[1])
		}
	}
	ids := map[string]string{
		"c4 id -q tree/":       run("", "id", "-q", tree+string(filepath.Separator)),
		"c4 id -q scan.c4m":    run("", "id", "-q", scanFile),
		"c4 id -q - < scan":    run(scanText, "id", "-q", "-"),
		"c4 id -q - < patched": run(flat, "id", "-q", "-"),
	}
	dirID := strings.TrimSpace(ids["c4 id -q tree/"])
	for form, id := range ids {
		if strings.TrimSpace(id) != dirID {
			t.Errorf("%s gives %s, want the tree's %s", form, strings.TrimSpace(id), dirID)
		}
	}
	if runtime.GOOS == "windows" {
		return // saved.c4m carries POSIX modes
	}
	saved, err := os.ReadFile(filepath.Join(vec, "saved.c4m"))
	if err != nil {
		t.Fatal(err)
	}
	if flat != string(saved) {
		t.Errorf("c4 id | c4 patch writes\n%s\nwant saved.c4m:\n%s", flat, saved)
	}
	root, err := os.ReadFile(filepath.Join(vec, "root-id.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if dirID != strings.TrimSpace(string(root)) {
		t.Errorf("description ID %s, want root-id.txt's %s", dirID, root)
	}
}

// buildEpochTree makes tree.txt's tree under dir: "<octal mode> <mtime ns>
// <path>" per line, a path ending in "/" is a directory, a file's bytes are
// its base name and a newline; times are set files first, then directories
// deepest first.
func buildEpochTree(t *testing.T, treeTxt, dir string) {
	t.Helper()
	b, err := os.ReadFile(treeTxt)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		mode os.FileMode
		ns   int64
		path string
	}
	var rows []row
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		p := strings.SplitN(l, " ", 3)
		m, _ := strconv.ParseUint(p[0], 8, 32)
		ns, _ := strconv.ParseInt(p[1], 10, 64)
		rows = append(rows, row{os.FileMode(m), ns, p[2]})
	}
	full := func(r row) string { return filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(r.path, "/"))) }
	isDir := func(r row) bool { return strings.HasSuffix(r.path, "/") }
	for _, r := range rows {
		if isDir(r) {
			os.MkdirAll(full(r), 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(full(r)), 0o755)
		if err := os.WriteFile(full(r), []byte(filepath.Base(full(r))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if isDir(rows[i]) != isDir(rows[j]) {
			return !isDir(rows[i])
		}
		return strings.Count(rows[i].path, "/") > strings.Count(rows[j].path, "/")
	})
	for _, r := range rows {
		os.Chmod(full(r), r.mode)
		ts := time.Unix(0, r.ns)
		if err := os.Chtimes(full(r), ts, ts); err != nil {
			t.Fatal(err)
		}
	}
}
