package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// verifiedFixture is a snapshotted tree: a.txt, sub/child.txt, its
// store, and the IDs a test needs to tamper with.
type verifiedFixture struct {
	bin, dir, storeDir string
	env                map[string]string
	snap               string // path of the full c4m
	aID, subID, rootID c4.ID  // a.txt content, sub/ record, root record
}

func newVerifiedFixture(t *testing.T) *verifiedFixture {
	t.Helper()
	f := &verifiedFixture{bin: buildC4(t), dir: t.TempDir()}
	f.storeDir = filepath.Join(f.dir, "store")
	f.env = map[string]string{"C4_STORE": f.storeDir}
	tree := filepath.Join(f.dir, "tree")
	if err := os.MkdirAll(filepath.Join(tree, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(tree, "a.txt"), "good content\n")
	writeTestFile(t, filepath.Join(tree, "b.txt"), "other\n")
	writeTestFile(t, filepath.Join(tree, "sub", "child.txt"), "child\n")

	out, stderr, code := runC4WithEnv(t, f.bin, f.env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("id -s exit %d: %s", code, stderr)
	}
	f.snap = filepath.Join(f.dir, "snap.c4m")
	writeTestFile(t, f.snap, out)

	m, err := c4m.Unmarshal([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	f.rootID = m.ComputeC4ID()
	for _, e := range m.Entries {
		switch e.Name {
		case "a.txt":
			f.aID = e.C4ID
		case "sub/":
			f.subID = e.C4ID
		}
	}
	if f.aID.IsNil() || f.subID.IsNil() {
		t.Fatalf("fixture IDs missing from:\n%s", out)
	}
	return f
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tamper replaces the stored object for id with data, wherever the store
// keeps it.
func (f *verifiedFixture) tamper(t *testing.T, id c4.ID, data string) {
	t.Helper()
	var found string
	filepath.Walk(f.storeDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Name() == id.String() {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatalf("object %s not in store", id)
	}
	os.Chmod(found, 0o644)
	writeTestFile(t, found, data)
}

func (f *verifiedFixture) readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A tampered store object must not be materialized: patch fails naming
// the ID, the destination file is untouched, and intact content in the
// same run still lands.
func TestPatchRefusesTamperedStoreObject(t *testing.T) {
	f := newVerifiedFixture(t)
	f.tamper(t, f.aID, "evil content\n")

	dst := filepath.Join(f.dir, "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dst, "a.txt"), "original\n")

	_, stderr, code := runC4WithEnv(t, f.bin, f.env, "patch", "-q", f.snap, dst)
	if code == 0 {
		t.Fatalf("patch exited 0 with a tampered store object; stderr:\n%s", stderr)
	}
	want := "store content for " + f.aID.String() + " does not match its ID"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr missing %q:\n%s", want, stderr)
	}
	if got := f.readFile(t, filepath.Join(dst, "a.txt")); got != "original\n" {
		t.Errorf("a.txt = %q, want untouched", got)
	}
	if got := f.readFile(t, filepath.Join(dst, "b.txt")); got != "other\n" {
		t.Errorf("b.txt = %q, want intact content materialized", got)
	}
}

// Intact store content materializes exactly as before.
func TestPatchIntactStoreObject(t *testing.T) {
	f := newVerifiedFixture(t)
	dst := filepath.Join(f.dir, "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runC4WithEnv(t, f.bin, f.env, "patch", "-q", f.snap, dst)
	if code != 0 {
		t.Fatalf("patch exit %d: %s", code, stderr)
	}
	if got := f.readFile(t, filepath.Join(dst, "a.txt")); got != "good content\n" {
		t.Errorf("a.txt = %q", got)
	}
	if got := f.readFile(t, filepath.Join(dst, "sub", "child.txt")); got != "child\n" {
		t.Errorf("sub/child.txt = %q", got)
	}
}

// evilRecord is valid c4m that a tampered sub-record could carry.
func evilRecord(t *testing.T) string {
	t.Helper()
	id := c4.Identify(strings.NewReader("evil\n"))
	return "-rw-r--r-- 2025-01-01T00:00:00Z 5 evil.txt " + id.String() + "\n"
}

// cat -r must not inline a sub-record whose bytes do not hash to the
// directory's ID.
func TestCatRecursiveRefusesTamperedSubRecord(t *testing.T) {
	f := newVerifiedFixture(t)
	out, _, code := runC4WithEnv(t, f.bin, f.env, "cat", "-r", f.rootID.String())
	if code != 0 || !strings.Contains(out, "child.txt") {
		t.Fatalf("intact cat -r: exit %d\n%s", code, out)
	}

	f.tamper(t, f.subID, evilRecord(t))
	out, stderr, code := runC4WithEnv(t, f.bin, f.env, "cat", "-r", f.rootID.String())
	if code == 0 {
		t.Fatalf("cat -r exited 0 with a tampered sub-record:\n%s", out)
	}
	if strings.Contains(out, "evil.txt") {
		t.Errorf("tampered sub-record was inlined:\n%s", out)
	}
	if !strings.Contains(stderr, f.subID.String()) || !strings.Contains(stderr, "does not match its ID") {
		t.Errorf("stderr does not name the mismatch:\n%s", stderr)
	}
}

// patch -r expands a root record through stored sub-records; a tampered
// root or sub-record must stop it before anything is written.
func TestPatchReverseRefusesTamperedRecords(t *testing.T) {
	for _, which := range []string{"root", "sub"} {
		f := newVerifiedFixture(t)
		id := f.rootID
		if which == "sub" {
			id = f.subID
		}
		f.tamper(t, id, evilRecord(t))

		dst := filepath.Join(f.dir, "dst")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(dst, "keep.txt"), "keep\n")

		_, stderr, code := runC4WithEnv(t, f.bin, f.env, "patch", "-q", "-r", f.rootID.String(), dst)
		if code == 0 {
			t.Fatalf("%s: patch -r exited 0 with a tampered record", which)
		}
		if !strings.Contains(stderr, id.String()) || !strings.Contains(stderr, "does not match its ID") {
			t.Errorf("%s: stderr does not name the mismatch:\n%s", which, stderr)
		}
		if got := f.readFile(t, filepath.Join(dst, "keep.txt")); got != "keep\n" {
			t.Errorf("%s: keep.txt = %q, destination was modified", which, got)
		}
		if _, err := os.Stat(filepath.Join(dst, "evil.txt")); err == nil {
			t.Errorf("%s: evil.txt materialized", which)
		}
	}
}

// diff -r loads the pre-patch manifest from the store; a tampered one
// must be refused, not decoded.
func TestDiffReverseRefusesTamperedManifest(t *testing.T) {
	f := newVerifiedFixture(t)
	dst := filepath.Join(f.dir, "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dst, "old.txt"), "old\n")

	changeset, stderr, code := runC4WithEnv(t, f.bin, f.env, "patch", f.snap, dst)
	if code != 0 {
		t.Fatalf("patch exit %d: %s", code, stderr)
	}
	csPath := filepath.Join(f.dir, "changeset.c4m")
	writeTestFile(t, csPath, changeset)
	oldID, err := c4.Parse(strings.TrimSpace(strings.SplitN(changeset, "\n", 2)[0]))
	if err != nil {
		t.Fatalf("changeset has no OldID line: %v\n%s", err, changeset)
	}

	if _, stderr, code := runC4WithEnv(t, f.bin, f.env, "diff", "-r", csPath, dst); code != 0 {
		t.Fatalf("intact diff -r exit %d: %s", code, stderr)
	}

	f.tamper(t, oldID, evilRecord(t))
	out, stderr, code := runC4WithEnv(t, f.bin, f.env, "diff", "-r", csPath, dst)
	if code == 0 {
		t.Fatalf("diff -r exited 0 with a tampered pre-patch manifest:\n%s", out)
	}
	if !strings.Contains(stderr, oldID.String()) || !strings.Contains(stderr, "does not match its ID") {
		t.Errorf("stderr does not name the mismatch:\n%s", stderr)
	}
}
