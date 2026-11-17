package store

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Avalanche-io/c4"
)

// A lookup tells "no copy at any level" from every other error. Through
// 2026-11-16 a failed stat of any kind read as absent: Open said "no such
// file" for an object it could not look for, Put published a second copy,
// and Remove reported nothing to remove (GC-DESIGN-R3 S1). Here the store
// is mid-split (the object one level above its place) and the stat at the
// object's level fails with an I/O error, through the stat seam, so every
// platform runs it.
func TestTreeStoreLookupTellsAbsentFromError(t *testing.T) {
	root := t.TempDir()
	s := splitTestStore(t, root, DefaultSplitThreshold)
	data := []byte("an object a stat cannot reach")
	id, err := s.Put(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	str := id.String()
	if err := os.Mkdir(filepath.Join(root, str[0:2]), 0o755); err != nil { // mid-split
		t.Fatal(err)
	}
	shallow := filepath.Join(root, str)
	eio := errors.New("input/output error (simulated)")
	r := splitTestStore(t, root, DefaultSplitThreshold)
	r.stat = func(p string) (os.FileInfo, error) {
		if p == shallow {
			return nil, &os.PathError{Op: "stat", Path: p, Err: eio}
		}
		return os.Stat(p)
	}
	isEIO := func(err error) bool { return errors.Is(err, eio) }

	if _, err := r.Lookup(id); !isEIO(err) {
		t.Errorf("Lookup: %v, want the stat's error", err)
	}
	if f, err := r.Open(id); !isEIO(err) {
		if f != nil {
			f.Close()
		}
		t.Errorf("Open: %v, want the stat's error, not not-found", err)
	}
	if _, err := r.Put(bytes.NewReader(data)); !isEIO(err) {
		t.Errorf("Put: %v, want the stat's error", err)
	}
	if err := r.Remove(id); !isEIO(err) {
		t.Errorf("Remove: %v, want the stat's error", err)
	}
	if r.Has(id) {
		t.Error("Has reports an object it could not look for")
	}
	if _, ok := r.ContentPath(id); ok {
		t.Error("ContentPath reports an object it could not look for")
	}
	if _, err := os.Stat(filepath.Join(root, str[0:2], str)); !os.IsNotExist(err) {
		t.Errorf("a second copy was published below: %v", err)
	}
	if _, err := os.Stat(shallow); err != nil {
		t.Errorf("the object is gone: %v", err)
	}

	// An ID no level holds is absent, and says so.
	other := c4.Identify(bytes.NewReader([]byte("never stored")))
	if _, err := r.Lookup(other); !os.IsNotExist(err) {
		t.Errorf("Lookup of an absent ID: %v, want not-exist", err)
	}
	if _, err := r.Open(other); !os.IsNotExist(err) {
		t.Errorf("Open of an absent ID: %v, want not-exist", err)
	}

	// A copy found anywhere wins over an error elsewhere on the path. The
	// copy is one level below the failing stat, and its place is deeper
	// still, so only the search finds it: past an error at a level.
	below := filepath.Join(root, str[0:2], str)
	if err := os.Rename(shallow, below); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, str[0:2], str[2:4]), 0o755); err != nil {
		t.Fatal(err)
	}
	if p, err := r.Lookup(id); err != nil || p != below {
		t.Errorf("Lookup past an error at a level: %q, %v; want %q", p, err, below)
	}
	f, err := r.Open(id)
	if err != nil {
		t.Fatalf("Open past an error at a level: %v", err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("Open read the wrong bytes")
	}

	// And past an error on one case variant of a shard directory: the stat
	// of the exact-case directory fails, and its other-case variant, which
	// the seam shows as the same directory, holds the copy.
	exact := filepath.Join(root, str[0:2])
	folded := filepath.Join(root, string([]byte{str[0] ^ 0x20, str[1]}))
	r.stat = func(p string) (os.FileInfo, error) {
		if p == exact {
			return nil, &os.PathError{Op: "stat", Path: p, Err: eio}
		}
		if p == folded || len(p) > len(folded) && p[:len(folded)+1] == folded+string(filepath.Separator) {
			return os.Stat(exact + p[len(folded):])
		}
		return os.Stat(p)
	}
	if p, err := r.Lookup(id); err != nil || p != filepath.Join(folded, str) {
		t.Errorf("Lookup past an error on a case variant: %q, %v; want %q", p, err, filepath.Join(folded, str))
	}
}

// The same with a real shard directory the user cannot read. On Windows
// chmod does not restrict a directory, and root reads anything, so it
// runs on Unix as a non-root user only.
func TestTreeStoreUnreadableShardIsNotAbsent(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions and a non-root user")
	}
	root := t.TempDir()
	s := splitTestStore(t, root, 4)
	var ids []c4.ID
	for i := 0; i < 24; i++ {
		ids = append(ids, putRandom(t, s))
	}
	// Pick an object whose leaf is a shard directory below c4/, and make
	// that directory unreadable and unsearchable.
	var id c4.ID
	var leaf string
	for _, x := range ids {
		p, err := s.Lookup(x)
		if err != nil {
			t.Fatal(err)
		}
		if d := filepath.Dir(p); d != filepath.Join(root, "c4") && d != root {
			id, leaf = x, d
			break
		}
	}
	if leaf == "" {
		t.Fatal("fixture: no object below the first shard level")
	}
	if err := os.Chmod(leaf, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(leaf, 0o755)

	r := splitTestStore(t, root, 4)
	if _, err := r.Lookup(id); err == nil || os.IsNotExist(err) {
		t.Errorf("Lookup in an unreadable shard: %v, want the permission error", err)
	}
	if f, err := r.Open(id); err == nil || os.IsNotExist(err) {
		if f != nil {
			f.Close()
		}
		t.Errorf("Open in an unreadable shard: %v, want the permission error", err)
	}
	if err := r.Remove(id); err == nil || os.IsNotExist(err) {
		t.Errorf("Remove in an unreadable shard: %v, want the permission error", err)
	}
	if r.Has(id) {
		t.Error("Has reports an object it could not look for")
	}
	// Objects elsewhere are unaffected.
	for _, x := range ids {
		p, err := r.Lookup(x)
		if err != nil {
			continue // in the unreadable shard
		}
		if filepath.Dir(p) == leaf {
			t.Fatalf("%s found inside the unreadable shard", x.String()[:12])
		}
	}
}
