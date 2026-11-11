package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// folds reports whether dir's volume takes a and b for one name.
func folds(t *testing.T, dir, a, b string) bool {
	t.Helper()
	d, err := os.MkdirTemp(dir, "fold-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)
	if err := os.WriteFile(filepath.Join(d, a), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ia, _ := os.Lstat(filepath.Join(d, a))
	ib, err := os.Lstat(filepath.Join(d, b))
	return err == nil && os.SameFile(ia, ib)
}

// writeFlatTarget stores each file's bytes and writes a one-level target
// c4m listing them, with modes this platform holds.
func writeFlatTarget(t *testing.T, work string, files map[string]string) (desc, storeDir string) {
	t.Helper()
	storeDir = filepath.Join(work, "store")
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if runtime.GOOS == "windows" {
		mode = 0o666
	}
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	target := c4m.NewManifest()
	for name, content := range files {
		id, err := s.Put(strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		target.AddEntry(&c4m.Entry{Name: name, Mode: mode, Timestamp: ts, Size: int64(len(content)), C4ID: id})
	}
	target.SortEntries()
	c4m.Normalize(target)
	var buf bytes.Buffer
	if err := c4m.NewEncoder(&buf).Encode(target); err != nil {
		t.Fatal(err)
	}
	desc = filepath.Join(work, "target.c4m")
	if err := os.WriteFile(desc, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return desc, storeDir
}

func listNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// A description that changes only a file's case is a rename, never a
// removal. On a folder that folds case, `Readme.md` on disk is the target's
// `readme.md` by identity; restore used to write the target's bytes into it
// and then remove it as unlisted, leaving the folder empty and failing its
// own verification (exit 3). Now the entry is kept, holds the target's
// bytes, is listed under exactly `readme.md`, and restore verifies (exit 0).
func TestRestoreCaseOnlyRenameKeepsTheFile(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "Readme.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := os.Lstat(filepath.Join(dest, "Readme.md"))
	if b, err := os.Lstat(filepath.Join(dest, "readme.md")); err != nil || !os.SameFile(a, b) {
		t.Skip("this volume tells Readme.md and readme.md apart")
	}

	bin := buildC4(t)
	desc, storeDir := writeFlatTarget(t, work, map[string]string{"readme.md": "new\n"})
	env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": storeDir}
	out, errOut, code := runC4WithEnv(t, bin, env, "restore", "--force", desc, dest)
	if code != 0 {
		t.Fatalf("restore exited %d, want 0\nstdout: %s\nstderr: %.600s", code, out, errOut)
	}
	if got := listNames(t, dest); !reflect.DeepEqual(got, []string{"readme.md"}) {
		t.Fatalf("folder lists %q, want exactly readme.md", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "readme.md")); string(b) != "new\n" {
		t.Errorf("readme.md = %q, want the target's bytes", b)
	}
}

// Two target names the destination folds together (case, or the NFC and
// NFD forms of é) are measured there: the byte-order last is written whole,
// under its own name and with its own bytes; the other is refused and
// named, never written, and restore verifies the rest and exits 2. Before,
// the second write went through the first's name, so one file carried the
// other's bytes. A volume that tells the names apart writes both (exit 0).
func TestRestoreRefusesNamesTheDestinationFolds(t *testing.T) {
	for _, pair := range [][2]string{{"README.md", "readme.md"}, {"é.txt", "é.txt"}} {
		work := t.TempDir()
		dest := filepath.Join(work, "dest")
		if err := os.Mkdir(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		folded := folds(t, work, pair[0], pair[1])
		lo, hi := pair[0], pair[1]
		if hi < lo {
			lo, hi = hi, lo
		}
		bin := buildC4(t)
		desc, storeDir := writeFlatTarget(t, work, map[string]string{lo: "low\n", hi: "high\n", "other.txt": "other\n"})
		env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": storeDir}
		out, errOut, code := runC4WithEnv(t, bin, env, "restore", "--force", desc, dest)
		if !folded {
			if code != 0 {
				t.Errorf("%q: restore exited %d on a volume that holds both, want 0\nstderr: %.600s", pair, code, errOut)
			}
			continue
		}
		if code != 2 {
			t.Errorf("%q: restore exited %d, want 2 (partial)\nstdout: %s\nstderr: %.600s", pair, code, out, errOut)
			continue
		}
		if !strings.Contains(errOut, "refused: "+lo+": ") {
			t.Errorf("%q: stderr does not name %q as refused:\n%s", pair, lo, errOut)
		}
		if strings.Contains(errOut, "verification failed") {
			t.Errorf("%q: the rest did not verify:\n%s", pair, errOut)
		}
		want := []string{hi, "other.txt"}
		sort.Strings(want)
		if got := listNames(t, dest); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: folder lists %q, want %q", pair, got, want)
		}
		if b, _ := os.ReadFile(filepath.Join(dest, hi)); string(b) != "high\n" {
			t.Errorf("%q: %q = %q, want its own bytes", pair, hi, b)
		}
	}
}

// A dry run writes nothing, not even a probe: it names sibling names that
// differ only in case as candidates, unmeasured, and the destination is
// left exactly as it was.
func TestRestoreDryRunWritesNoProbe(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := buildC4(t)
	desc, storeDir := writeFlatTarget(t, work, map[string]string{"README.md": "a\n", "readme.md": "b\n"})
	env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": storeDir}
	out, errOut, code := runC4WithEnv(t, bin, env, "restore", desc, dest)
	if code != 0 {
		t.Fatalf("dry run exited %d, want 0\nstdout: %s\nstderr: %.600s", code, out, errOut)
	}
	if !strings.Contains(errOut, "may collide: README.md with readme.md (not measured)") {
		t.Errorf("dry run does not name the case pair as a candidate:\n%s", errOut)
	}
	if got := listNames(t, dest); len(got) != 0 {
		t.Errorf("dry run left %q in the destination, want nothing", got)
	}
}
