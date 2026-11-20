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

// A folder that already exists read-only (0555) in the destination is
// opened for the plan's work inside it: a file added, one removed. It then
// gets its recorded mode: 0555 again when the description keeps it so,
// 0755 when the description opens it.
func TestApplyWritesIntoAnExistingReadOnlyFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows folder has no owner-write bit to withhold")
	}
	for _, recorded := range []os.FileMode{0o555, 0o755} {
		recorded := recorded
		t.Run(recorded.String(), func(t *testing.T) {
			ts := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
			src := store.NewRAM()
			id, err := src.Put(strings.NewReader("new\n"))
			if err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			unlock(t, dest)
			ro := filepath.Join(dest, "ro")
			if err := os.MkdirAll(ro, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := ioutil.WriteFile(filepath.Join(ro, "old.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(ro, 0o555); err != nil {
				t.Fatal(err)
			}
			target := c4m.NewManifest()
			target.AddEntry(&c4m.Entry{Name: "ro/", Mode: os.ModeDir | recorded, Timestamp: ts, Size: -1, Depth: 0})
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
			info, err := os.Stat(ro)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != recorded {
				t.Errorf("ro: mode %v, want the recorded %v", info.Mode().Perm(), recorded)
			}
			if _, err := os.Stat(filepath.Join(ro, "old.txt")); !os.IsNotExist(err) {
				t.Errorf("old.txt should be removed (stat err %v)", err)
			}
			if got, err := ioutil.ReadFile(filepath.Join(ro, "new.txt")); err != nil || string(got) != "new\n" {
				t.Errorf("new.txt: %q, %v", got, err)
			}
		})
	}
}

// Into a target folder that does not exist yet, a read-only subfolder's
// mode stays on the subfolder: the target is made with the default mode
// (it is not in the description), its top-level file is written, and the
// subfolder is 0555 with its contents. Through 2026-11-20 the target took
// the subfolder's 0555 (MkdirAll gives every folder it makes the one mode
// it is passed), and nothing at all was written.
func TestApplyIntoANewTargetLeavesItsModeAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows folder has no owner-write bit to withhold")
	}
	for _, mode := range []os.FileMode{0o555, 0o500} {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) { newTargetModeAlone(t, mode) })
	}
}

func newTargetModeAlone(t *testing.T, mode os.FileMode) {
	ts := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	src := store.NewRAM()
	put := func(s string) (c4m.Entry, error) {
		id, err := src.Put(strings.NewReader(s))
		return c4m.Entry{C4ID: id, Size: int64(len(s))}, err
	}
	top, _ := put("top\n")
	f, _ := put("f\n")
	target := c4m.NewManifest()
	for _, e := range []*c4m.Entry{
		{Name: "top.txt", Mode: 0o644, Timestamp: ts, Size: top.Size, C4ID: top.C4ID, Depth: 0},
		{Name: "ro/", Mode: os.ModeDir | mode, Timestamp: ts, Size: -1, Depth: 0},
		{Name: "f.txt", Mode: 0o644, Timestamp: ts, Size: f.Size, C4ID: f.C4ID, Depth: 1},
	} {
		target.AddEntry(e)
	}
	parent := t.TempDir()
	unlock(t, parent)
	dest := filepath.Join(parent, "new", "target") // neither exists yet
	ref := filepath.Join(parent, "ref")
	if err := os.MkdirAll(ref, 0o755); err != nil { // the default, as the umask makes it
		t.Fatal(err)
	}
	refInfo, _ := os.Stat(ref)

	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Apply(plan, dest)
	if err != nil || len(res.Errors) > 0 {
		t.Fatalf("apply: %v %v", err, res.Errors)
	}
	for _, p := range []string{filepath.Join(parent, "new"), dest} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != refInfo.Mode().Perm() {
			t.Errorf("%s: mode %v, want the default %v (it is not in the description)", p, info.Mode().Perm(), refInfo.Mode().Perm())
		}
	}
	if got, err := ioutil.ReadFile(filepath.Join(dest, "top.txt")); err != nil || string(got) != "top\n" {
		t.Errorf("top.txt: %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dest, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Errorf("ro: mode %v, want the recorded %v", info.Mode().Perm(), mode)
	}
	if err := os.Chmod(filepath.Join(dest, "ro"), 0o755); err != nil { // to read inside
		t.Fatal(err)
	}
	if got, err := ioutil.ReadFile(filepath.Join(dest, "ro", "f.txt")); err != nil || string(got) != "f\n" {
		t.Errorf("ro/f.txt: %q, %v", got, err)
	}
}

// Distribute copies a read-only source folder (0555) with its contents,
// and gives the copy the source's mode after them.
func TestDistributeCopiesAReadOnlyFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows folder has no owner-write bit to withhold")
	}
	srcDir := t.TempDir()
	unlock(t, srcDir)
	if err := os.MkdirAll(filepath.Join(srcDir, "ro", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, c := range map[string]string{"ro/f.txt": "f\n", "ro/sub/g.txt": "g\n"} {
		if err := ioutil.WriteFile(filepath.Join(srcDir, filepath.FromSlash(p)), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(srcDir, "ro"), 0o555); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	unlock(t, parent)
	dst := filepath.Join(parent, "copy")
	res, err := Distribute(srcDir, ToDir(dst))
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range res.Targets {
		if len(tr.Errors) > 0 {
			t.Fatalf("distribute: %v", tr.Errors)
		}
	}
	info, err := os.Stat(filepath.Join(dst, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Errorf("ro: mode %v, want the source's 0555", info.Mode().Perm())
	}
	for p, want := range map[string]string{"ro/f.txt": "f\n", "ro/sub/g.txt": "g\n"} {
		if got, err := ioutil.ReadFile(filepath.Join(dst, filepath.FromSlash(p))); err != nil || string(got) != want {
			t.Errorf("%s: %q, %v", p, got, err)
		}
	}
}

// A read-only folder the plan only writes into, with nothing to set on the
// folder itself (the target root, which no description names, or a folder
// recorded with a null time and its own mode), is opened for the work and
// set back to its own mode afterwards.
func TestApplySetsAnOpenedFolderBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows folder has no owner-write bit to withhold")
	}
	src := store.NewRAM()
	id, err := src.Put(strings.NewReader("new\n"))
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)

	t.Run("target root", func(t *testing.T) {
		dest := t.TempDir()
		unlock(t, dest)
		if err := os.Chmod(dest, 0o555); err != nil {
			t.Fatal(err)
		}
		target := c4m.NewManifest()
		target.AddEntry(&c4m.Entry{Name: "new.txt", Mode: 0o644, Timestamp: ts, Size: 4, C4ID: id, Depth: 0})
		r := New(WithSource(src))
		plan, err := r.Plan(target, dest)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := r.Apply(plan, dest); err != nil || len(res.Errors) > 0 {
			t.Fatalf("apply: %v %v", err, res.Errors)
		}
		info, _ := os.Stat(dest)
		if info.Mode().Perm() != 0o555 {
			t.Errorf("target root: mode %v, want its own 0555 set back", info.Mode().Perm())
		}
		if _, err := os.Stat(filepath.Join(dest, "new.txt")); err != nil {
			t.Errorf("new.txt: %v", err)
		}
	})

	t.Run("null-time folder", func(t *testing.T) {
		dest := t.TempDir()
		unlock(t, dest)
		ro := filepath.Join(dest, "ro")
		if err := os.MkdirAll(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		target := c4m.NewManifest()
		target.AddEntry(&c4m.Entry{Name: "ro/", Mode: os.ModeDir | 0o555, Timestamp: c4m.NullTimestamp(), Size: -1, Depth: 0})
		target.AddEntry(&c4m.Entry{Name: "new.txt", Mode: 0o644, Timestamp: ts, Size: 4, C4ID: id, Depth: 1})
		r := New(WithSource(src))
		plan, err := r.Plan(target, dest)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := r.Apply(plan, dest); err != nil || len(res.Errors) > 0 {
			t.Fatalf("apply: %v %v", err, res.Errors)
		}
		info, _ := os.Stat(ro)
		if info.Mode().Perm() != 0o555 {
			t.Errorf("ro: mode %v, want 0555 set back", info.Mode().Perm())
		}
	})
}
