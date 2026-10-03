package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// A store built on a case-insensitive filesystem (default macOS APFS) can
// hold objects whose IDs differ only in case at some level in a single
// shard directory named by whichever ID created it first ("c4/1J" holding
// "c41j..." objects). Copied to a case-sensitive filesystem, the
// exact-case probe for "1j" misses. These tests build such layouts and
// require every read path to still find the object.

// caseSensitiveStat emulates a case-sensitive filesystem under root on any
// host: every path component must match an on-disk name exactly. On a
// case-sensitive host it is equivalent to os.Stat; on macOS it forces the
// exact-case probe to miss where a case-variant directory exists.
func caseSensitiveStat(root string) func(string) (os.FileInfo, error) {
	return func(p string) (os.FileInfo, error) {
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return os.Stat(p)
		}
		dir := root
		if rel != "." {
			for _, part := range strings.Split(rel, string(filepath.Separator)) {
				entries, err := os.ReadDir(dir)
				if err != nil {
					return nil, err
				}
				found := false
				for _, e := range entries {
					if e.Name() == part {
						found = true
						break
					}
				}
				if !found {
					return nil, &os.PathError{Op: "stat", Path: p, Err: os.ErrNotExist}
				}
				dir = filepath.Join(dir, part)
			}
		}
		return os.Stat(p)
	}
}

// toggleCase flips the case of every ASCII letter in s.
func toggleCase(s string) string {
	b := []byte(s)
	for i, c := range b {
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') {
			b[i] = c ^ 0x20
		}
	}
	return string(b)
}

func hasLetter(s string) bool {
	return toggleCase(s) != s
}

// contentWithLetters returns content whose ID has an ASCII letter in each
// of the first `levels` shard segments after "c4".
func contentWithLetters(t *testing.T, levels int) ([]byte, c4.ID) {
	t.Helper()
	for i := 0; i < 100000; i++ {
		data := []byte(fmt.Sprintf("case-collision %d", i))
		id := c4.Identify(bytes.NewReader(data))
		str := id.String()
		ok := true
		for l := 0; l < levels; l++ {
			if !hasLetter(str[2+2*l : 4+2*l]) {
				ok = false
				break
			}
		}
		if ok {
			return data, id
		}
	}
	t.Fatal("no suitable content found")
	return nil, c4.ID{}
}

// plantCollided writes the object for data under shard directories whose
// names are the case-toggled segments of its ID, for `levels` levels
// below "c4". It returns the object's on-disk path.
func plantCollided(t *testing.T, root string, data []byte, id c4.ID, levels int) string {
	t.Helper()
	str := id.String()
	dir := filepath.Join(root, "c4")
	for l := 0; l < levels; l++ {
		dir = filepath.Join(dir, toggleCase(str[2+2*l:4+2*l]))
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, str)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func countObjects(t *testing.T, s *TreeStore) int {
	t.Helper()
	n := 0
	if err := s.Walk(func(c4.ID, int64) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	return n
}

// checkCollidedReads asserts every read path finds the planted object.
func checkCollidedReads(t *testing.T, s *TreeStore, id c4.ID, data []byte, want string) {
	t.Helper()
	if !s.Has(id) {
		t.Fatal("Has: object under case-variant shard reported missing")
	}
	rc, err := s.Open(id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("Open: content mismatch (err %v)", err)
	}
	p, ok := s.ContentPath(id)
	if !ok {
		t.Fatal("ContentPath: object under case-variant shard reported missing")
	}
	if p != want {
		t.Fatalf("ContentPath = %s, want %s", p, want)
	}

	// A write of the same bytes must find the existing object and skip.
	before := countObjects(t, s)
	pid, err := s.Put(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if pid != id {
		t.Fatal("Put returned a different ID")
	}
	if n := countObjects(t, s); n != before {
		t.Fatalf("Put wrote a duplicate object: %d objects, want %d", n, before)
	}
	if _, err := s.Create(id); !os.IsExist(err) {
		t.Fatalf("Create: want ErrExist, got %v", err)
	}

	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.Has(id) {
		t.Fatal("Has after Remove: still present")
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("object file still on disk after Remove: %v", err)
	}
}

// TestTreeStoreCaseCollidedShardFallback forces case-sensitive lookups on
// any host and proves the fallback finds "c41j..." under a dir named "1J",
// at one and two levels of collision.
func TestTreeStoreCaseCollidedShardFallback(t *testing.T) {
	for _, levels := range []int{1, 2} {
		t.Run(fmt.Sprintf("levels=%d", levels), func(t *testing.T) {
			root := t.TempDir()
			s, err := NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			s.stat = caseSensitiveStat(root)

			data, id := contentWithLetters(t, levels)
			want := plantCollided(t, root, data, id, levels)

			// The exact-case trie path must miss under the emulation,
			// or this test is not exercising the fallback.
			if _, err := s.stat(s.path(id)); err == nil {
				t.Fatal("exact-case probe hit; fallback not exercised")
			}
			checkCollidedReads(t, s, id, data, want)
		})
	}
}

// TestTreeStoreCaseCollidedShardBatch covers batch mode: a pending write of
// bytes already present under a case-variant shard must not publish a
// duplicate at Sync.
func TestTreeStoreCaseCollidedShardBatch(t *testing.T) {
	root := t.TempDir()
	s, err := NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	s.stat = caseSensitiveStat(root)
	s.SetSyncMode(SyncBatch)

	data, id := contentWithLetters(t, 2)
	plantCollided(t, root, data, id, 2)
	if _, err := s.Put(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if n := countObjects(t, s); n != 1 {
		t.Fatalf("batch Put+Sync wrote a duplicate: %d objects", n)
	}
}

// TestTreeStoreCaseCollidedShardLayout uses the real filesystem with no
// emulation. On a case-sensitive host (Linux CI) the exact-case probe for
// the ID's segment genuinely misses; on a case-insensitive host it hits.
// Either way every read path must find the object.
func TestTreeStoreCaseCollidedShardLayout(t *testing.T) {
	for _, levels := range []int{1, 2} {
		t.Run(fmt.Sprintf("levels=%d", levels), func(t *testing.T) {
			root := t.TempDir()
			s, err := NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			data, id := contentWithLetters(t, levels)
			planted := plantCollided(t, root, data, id, levels)
			want := planted
			if p := s.path(id); p != planted {
				if _, err := os.Stat(p); err == nil {
					want = p // case-insensitive host: the exact-case path resolves
				}
			}
			checkCollidedReads(t, s, id, data, want)
		})
	}
}

// TestTreeStoreCaseMissStaysMissing: the fallback must not invent objects.
func TestTreeStoreCaseMissStaysMissing(t *testing.T) {
	root := t.TempDir()
	s, err := NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	s.stat = caseSensitiveStat(root)
	_, id := contentWithLetters(t, 2)
	str := id.String()
	// Variant shard dirs exist, but the object file is absent.
	dir := filepath.Join(root, "c4", toggleCase(str[2:4]), toggleCase(str[4:6]))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if s.Has(id) {
		t.Fatal("Has reported an absent object")
	}
	if _, ok := s.ContentPath(id); ok {
		t.Fatal("ContentPath reported an absent object")
	}
	if _, err := s.Open(id); !os.IsNotExist(err) {
		t.Fatalf("Open: want not-exist, got %v", err)
	}
	if err := s.Remove(id); !os.IsNotExist(err) {
		t.Fatalf("Remove: want not-exist, got %v", err)
	}
}

func TestTreeStoreFoldVariants(t *testing.T) {
	cases := map[string]int{"12": 1, "1j": 2, "aB": 4, "zz": 4}
	for seg, n := range cases {
		v := foldVariants(seg)
		if len(v) != n || v[0] != seg {
			t.Fatalf("foldVariants(%q) = %v, want %d variants, exact first", seg, v, n)
		}
		seen := map[string]bool{}
		for _, x := range v {
			if !strings.EqualFold(x, seg) || seen[x] {
				t.Fatalf("foldVariants(%q) = %v: bad or duplicate variant %q", seg, v, x)
			}
			seen[x] = true
		}
	}
}
