package scan

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

const subsecondVector = "../c4m/testdata/subsecond-vector"

// buildSubsecondTree makes the tree tree.txt describes under dir: each line
// is "<octal mode> <mtime ns> <path>", a path ending in "/" is a directory,
// and a file's bytes are its base name and a newline. Times are set once
// every entry exists, files first, then directories deepest first, so a
// child's time never moves its parent's afterwards.
func buildSubsecondTree(t *testing.T, dir string) {
	t.Helper()
	f, err := os.Open(filepath.Join(subsecondVector, "tree.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type row struct {
		mode  os.FileMode
		ns    int64
		path  string
		isDir bool
	}
	var rows []row
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), " ", 3)
		mode, err := strconv.ParseUint(parts[0], 8, 32)
		if err != nil {
			t.Fatal(err)
		}
		ns, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row{os.FileMode(mode), ns, parts[2], strings.HasSuffix(parts[2], "/")})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	full := func(r row) string {
		return filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(r.path, "/")))
	}
	for _, r := range rows {
		if r.isDir {
			if err := os.MkdirAll(full(r), 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		os.MkdirAll(filepath.Dir(full(r)), 0o755)
		if err := os.WriteFile(full(r), []byte(filepath.Base(full(r))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].isDir != rows[j].isDir {
			return !rows[i].isDir
		}
		return strings.Count(rows[i].path, "/") > strings.Count(rows[j].path, "/")
	})
	for _, r := range rows {
		if err := os.Chmod(full(r), r.mode); err != nil {
			t.Fatal(err)
		}
		ts := time.Unix(0, r.ns)
		if err := os.Chtimes(full(r), ts, ts); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSubsecondVector is the shared conformance row for a scan's
// sub-second times (c4m/testdata/subsecond-vector/README.md): a tree whose
// mtimes fall inside a second, scanned and diffed against its own saved
// c4m, gives an empty patch, and its times are the seconds saved.c4m
// shows, floored.
func TestSubsecondVector(t *testing.T) {
	savedText, err := os.ReadFile(filepath.Join(subsecondVector, "saved.c4m"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(subsecondVector, "root-id.txt"))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := c4m.Unmarshal(savedText)
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.ComputeC4ID().String(); got != strings.TrimSpace(string(want)) {
		t.Fatalf("saved.c4m resolves to %s, want root-id.txt's %s", got, want)
	}

	dir := t.TempDir()
	buildSubsecondTree(t, dir)
	scanned, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	text, err := c4m.Marshal(scanned)
	if err != nil {
		t.Fatal(err)
	}
	own, err := c4m.Unmarshal(text)
	if err != nil {
		t.Fatal(err)
	}

	empty := func(name string, base *c4m.Manifest) {
		t.Helper()
		pr := c4m.PatchDiff(base, scanned)
		if n := len(pr.Patch.Entries); n != 0 {
			patch, _ := c4m.Marshal(pr.Patch)
			t.Fatalf("diff of the scan against %s carries %d entries, want none:\n%s", name, n, patch)
		}
	}
	// The scan's own saved c4m: holds on every platform.
	empty("its own saved c4m", own)

	// The times are the format's: whole seconds, floored, on every platform.
	savedTimes := map[string]string{}
	for _, e := range saved.Entries {
		savedTimes[e.Name] = e.Timestamp.UTC().Format(time.RFC3339)
	}
	for _, e := range own.Entries {
		if got := e.Timestamp.UTC().Format(time.RFC3339); got != savedTimes[e.Name] {
			t.Errorf("%s: scanned time %s, want %s", e.Name, got, savedTimes[e.Name])
		}
	}

	if runtime.GOOS == "windows" {
		return // saved.c4m carries POSIX modes
	}
	if !bytes.Equal(text, savedText) {
		t.Fatalf("the scan encodes differently from saved.c4m:\n%s\nwant:\n%s", text, savedText)
	}
	empty("saved.c4m", saved)
}
