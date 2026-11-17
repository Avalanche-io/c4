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
// and a directory's description ID differed from its c4m's. 1.0.27's
// backport of master's fix.
func TestEpochSecondVectorCLI(t *testing.T) {
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "epoch-second-vector")
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}
	tree := filepath.Join(dir, "tree")
	buildEpochTree(t, filepath.Join(vec, "tree.txt"), tree)

	run := func(stdin string, args ...string) string {
		t.Helper()
		var out, stderr string
		var code int
		if stdin == "" {
			out, stderr, code = runC4WithEnv(t, bin, env, args...)
		} else {
			f := filepath.Join(dir, "stdin.c4m")
			if err := os.WriteFile(f, []byte(stdin), 0o644); err != nil {
				t.Fatal(err)
			}
			args = append(args[:len(args)-1], f)
			out, stderr, code = runC4WithEnv(t, bin, env, args...)
		}
		if code != 0 {
			t.Fatalf("c4 %v: exit %d: %s", args, code, stderr)
		}
		return out
	}

	scanText := run("", "id", tree+string(filepath.Separator))
	flat := run(scanText, "patch", "-")
	for _, line := range strings.Split(strings.TrimSpace(flat), "\n") {
		f := strings.Fields(line)
		if (f[3] == "half.txt" || f[3] == "in.txt") && f[1] != "-" {
			t.Errorf("%s is written %q, want -", f[3], f[1])
		}
	}
	// 1.0.x's c4 id -q prints nothing; the description IDs are pinned by
	// TestEpochSecondVector in scan. Here: the scan's chain resolves.
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
