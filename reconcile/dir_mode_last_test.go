package reconcile

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// unlock makes every folder under root writable again so the test's
// temporary folder can be removed.
func unlock(t *testing.T, root string) {
	t.Cleanup(func() {
		filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
}

// A folder's recorded mode is set after its children are written, so a
// folder recorded without owner write (0555, a read-only delivery folder,
// or 0000) still receives its file and its subfolder. Through 2026-11-20
// the folder was made with its recorded mode first, and every child was
// refused: "permission denied".
func TestApplyWritesIntoAFolderRecordedReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows folder has no owner-write bit to withhold")
	}
	for _, mode := range []os.FileMode{0o555, 0o500, 0o000} {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) {
			ts := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
			src := store.NewRAM()
			put := func(s string) c4m.Entry {
				id, err := src.Put(strings.NewReader(s))
				if err != nil {
					t.Fatal(err)
				}
				return c4m.Entry{C4ID: id, Size: int64(len(s))}
			}
			f, g := put("f\n"), put("g\n")
			target := c4m.NewManifest()
			for _, e := range []*c4m.Entry{
				{Name: "ro/", Mode: os.ModeDir | mode, Timestamp: ts, Size: -1, Depth: 0},
				{Name: "f.txt", Mode: 0o644, Timestamp: ts, Size: f.Size, C4ID: f.C4ID, Depth: 1},
				{Name: "sub/", Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1, Depth: 1},
				{Name: "g.txt", Mode: 0o644, Timestamp: ts, Size: g.Size, C4ID: g.C4ID, Depth: 2},
			} {
				target.AddEntry(e)
			}

			dest := t.TempDir()
			unlock(t, dest)
			r := New(WithSource(src))
			plan, err := r.Plan(target, dest)
			if err != nil {
				t.Fatal(err)
			}
			res, err := r.Apply(plan, dest)
			if err != nil || len(res.Errors) > 0 {
				t.Fatalf("apply: %v %v", err, res.Errors)
			}
			check := func(p string, want os.FileMode) {
				t.Helper()
				info, err := os.Stat(filepath.Join(dest, p))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != want {
					t.Errorf("%s: mode %v, want the recorded %v", p, info.Mode().Perm(), want)
				}
				if !info.ModTime().UTC().Equal(ts) {
					t.Errorf("%s: mtime %v, want the recorded %v", p, info.ModTime().UTC(), ts)
				}
			}
			check("ro", mode)
			// Open ro to look inside (a chmod leaves the mtime as it is).
			if err := os.Chmod(filepath.Join(dest, "ro"), 0o755); err != nil {
				t.Fatal(err)
			}
			check("ro/sub", 0o755)
			for p, want := range map[string]string{"ro/f.txt": "f\n", "ro/sub/g.txt": "g\n"} {
				got, err := ioutil.ReadFile(filepath.Join(dest, p))
				if err != nil || string(got) != want {
					t.Errorf("%s: %q, %v; want %q", p, got, err, want)
				}
			}
		})
	}
}

// A folder that exists and is recorded with a mode without owner write
// gets that mode only after the files written into it and removed from it:
// set first, it refused both.
func TestApplySetsAnExistingFoldersModeLast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows folder has no owner-write bit to withhold")
	}
	// With a recorded time the folder also gets a chtimes; with a null one
	// the chmod is the only operation that sets its mode.
	for _, ts := range []time.Time{time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), c4m.NullTimestamp()} {
		ts := ts
		t.Run(ts.Format("2006"), func(t *testing.T) { existingFolderModeLast(t, ts) })
	}
}

func existingFolderModeLast(t *testing.T, ts time.Time) {
	src := store.NewRAM()
	id, err := src.Put(strings.NewReader("new\n"))
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	unlock(t, dest)
	if err := os.MkdirAll(filepath.Join(dest, "ro"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dest, "ro", "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "ro/", Mode: os.ModeDir | 0o555, Timestamp: ts, Size: -1, Depth: 0})
	target.AddEntry(&c4m.Entry{Name: "new.txt", Mode: 0o644, Timestamp: ts, Size: 4, C4ID: id, Depth: 1})

	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Apply(plan, dest)
	if err != nil || len(res.Errors) > 0 {
		t.Fatalf("apply: %v %v", err, res.Errors)
	}
	info, err := os.Stat(filepath.Join(dest, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Errorf("ro: mode %v, want the recorded 0555", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dest, "ro", "old.txt")); !os.IsNotExist(err) {
		t.Errorf("old.txt should be removed (stat err %v)", err)
	}
	if got, err := ioutil.ReadFile(filepath.Join(dest, "ro", "new.txt")); err != nil || string(got) != "new\n" {
		t.Errorf("new.txt: %q, %v", got, err)
	}
}
