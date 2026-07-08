package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// snapshotDir runs `c4 id -s <dir>` against the given store and writes the
// resulting manifest next to the store. Returns the manifest path and the
// set of C4 IDs appearing in it.
func snapshotDir(t *testing.T, bin, storeDir, dir, name string) (string, map[c4.ID]bool) {
	t.Helper()
	out, stderr, code := runC4WithEnv(t, bin, map[string]string{"C4_STORE": storeDir}, "id", "-s", dir)
	if code != 0 {
		t.Fatalf("id -s %s exit %d: %s", dir, code, stderr)
	}
	path := filepath.Join(filepath.Dir(storeDir), name)
	if err := os.WriteFile(path, []byte(out), 0644); err != nil {
		t.Fatal(err)
	}
	return path, idSet([]byte(out))
}

func idSet(data []byte) map[c4.ID]bool {
	set := make(map[c4.ID]bool)
	for _, id := range scanIDs(data) {
		set[id] = true
	}
	return set
}

func countObjects(t *testing.T, storeDir string) int {
	t.Helper()
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	err = s.Walk(func(id c4.ID, size int64) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGCKeepsExactlyReachable(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	dirA := filepath.Join(dir, "a")
	dirB := filepath.Join(dir, "b")
	writeTree(t, dirA, map[string]string{
		"a1.txt":     "alpha one",
		"shared.txt": "shared payload",
		"sub/a2.txt": "alpha two",
	})
	writeTree(t, dirB, map[string]string{
		"b1.txt":     "bravo one",
		"shared.txt": "shared payload",
	})

	aPath, aIDs := snapshotDir(t, bin, storeDir, dirA, "a.c4m")
	_, bIDs := snapshotDir(t, bin, storeDir, dirB, "b.c4m")

	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	// Sanity: everything both snapshots reference is in the store.
	for id := range aIDs {
		if !s.Has(id) {
			t.Fatalf("missing stored object %s from a.c4m", id)
		}
	}
	for id := range bIDs {
		if !s.Has(id) {
			t.Fatalf("missing stored object %s from b.c4m", id)
		}
	}
	before := countObjects(t, storeDir)

	// Dry run must delete nothing.
	out, stderr, code := runC4WithEnv(t, bin, env, "gc", aPath)
	if code != 0 {
		t.Fatalf("gc dry run exit %d: %s", code, stderr)
	}
	if !strings.Contains(out, "Dry run") {
		t.Fatalf("dry run output missing marker: %s", out)
	}
	if got := countObjects(t, storeDir); got != before {
		t.Fatalf("dry run changed store: %d objects, want %d", got, before)
	}

	// Force: exactly the objects reachable from a.c4m survive.
	out, stderr, code = runC4WithEnv(t, bin, env, "gc", "--force", aPath)
	if code != 0 {
		t.Fatalf("gc --force exit %d: %s", code, stderr)
	}
	if !strings.Contains(out, "deleted") {
		t.Fatalf("force output missing deleted summary: %s", out)
	}
	for id := range aIDs {
		if !s.Has(id) {
			t.Fatalf("gc deleted reachable object %s", id)
		}
	}
	for id := range bIDs {
		if aIDs[id] {
			continue
		}
		if s.Has(id) {
			t.Fatalf("gc kept unreachable object %s", id)
		}
	}
	if got := countObjects(t, storeDir); got != len(aIDs) {
		t.Fatalf("store has %d objects after gc, want exactly %d", got, len(aIDs))
	}

	// Idempotent: a second force run finds no garbage.
	out, stderr, code = runC4WithEnv(t, bin, env, "gc", "--force", aPath)
	if code != 0 {
		t.Fatalf("second gc --force exit %d: %s", code, stderr)
	}
	if !strings.Contains(out, "deleted 0 objects") {
		t.Fatalf("second gc --force deleted something: %s", out)
	}
}

func TestGCRefusesEmptyKeepSet(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	dirA := filepath.Join(dir, "a")
	writeTree(t, dirA, map[string]string{"a1.txt": "alpha one"})
	snapshotDir(t, bin, storeDir, dirA, "a.c4m")
	before := countObjects(t, storeDir)
	if before == 0 {
		t.Fatal("expected populated store")
	}

	// No arguments is a usage error.
	_, _, code := runC4WithEnv(t, bin, env, "gc")
	if code == 0 {
		t.Fatal("gc with no roots must fail")
	}
	_, _, code = runC4WithEnv(t, bin, env, "gc", "--force")
	if code == 0 {
		t.Fatal("gc --force with no roots must fail")
	}

	// A structure-mode manifest parses but references no IDs: refuse.
	out, _, c := runC4WithEnv(t, bin, env, "id", "-m", "s", dirA)
	if c != 0 {
		t.Fatalf("id -m s exit %d", c)
	}
	structPath := filepath.Join(dir, "structure.c4m")
	if err := os.WriteFile(structPath, []byte(out), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runC4WithEnv(t, bin, env, "gc", "--force", structPath)
	if code == 0 {
		t.Fatal("gc with empty keep-set must fail")
	}
	if !strings.Contains(stderr, "Refusing") {
		t.Fatalf("expected refusal message, got: %s", stderr)
	}

	if got := countObjects(t, storeDir); got != before {
		t.Fatalf("refused gc changed store: %d objects, want %d", got, before)
	}
}

func TestGCParseErrorIsFatal(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	dirA := filepath.Join(dir, "a")
	writeTree(t, dirA, map[string]string{"a1.txt": "alpha one"})
	aPath, _ := snapshotDir(t, bin, storeDir, dirA, "a.c4m")
	before := countObjects(t, storeDir)

	junk := filepath.Join(dir, "junk.c4m")
	if err := os.WriteFile(junk, []byte("this is not a c4m file {\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runC4WithEnv(t, bin, env, "gc", "--force", junk)
	if code == 0 {
		t.Fatal("gc with unparseable root must fail")
	}

	// A parse failure in any root aborts before anything is deleted, even
	// when other roots are valid.
	_, _, code = runC4WithEnv(t, bin, env, "gc", "--force", aPath, junk)
	if code == 0 {
		t.Fatal("gc with one unparseable root must fail")
	}
	if got := countObjects(t, storeDir); got != before {
		t.Fatalf("failed gc changed store: %d objects, want %d", got, before)
	}
}

func TestGCClosureFollowsStoredDescriptions(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	dirA := filepath.Join(dir, "a")
	dirB := filepath.Join(dir, "b")
	writeTree(t, dirA, map[string]string{
		"a1.txt":     "alpha one",
		"sub/a2.txt": "alpha two",
	})
	writeTree(t, dirB, map[string]string{"b1.txt": "bravo one"})

	v1Path, v1IDs := snapshotDir(t, bin, storeDir, dirA, "v1.c4m")
	_, bIDs := snapshotDir(t, bin, storeDir, dirB, "b.c4m")

	// Store the canonical v1 manifest itself, as `c4 id -s v1.c4m` would.
	v1Data, err := os.ReadFile(v1Path)
	if err != nil {
		t.Fatal(err)
	}
	canonical, m := canonicalizeC4mBytes(v1Data)
	if m == nil {
		t.Fatal("v1.c4m did not canonicalize")
	}
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	baseID, err := s.Put(bytes.NewReader(canonical))
	if err != nil {
		t.Fatal(err)
	}

	// Change a1.txt and snapshot v2: the original a1 content is now
	// reachable only through the stored v1 manifest.
	writeTree(t, dirA, map[string]string{
		"a1.txt": "alpha one revised",
		"a3.txt": "alpha three",
	})
	v2Path, v2IDs := snapshotDir(t, bin, storeDir, dirA, "v2.c4m")
	v2Data, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatal(err)
	}

	// Chain file: external base reference (bare ID line) + v2 entries.
	// The base's own content is reachable only through the stored manifest.
	chain := baseID.String() + "\n" + string(v2Data)
	chainPath := filepath.Join(dir, "chain.c4m")
	if err := os.WriteFile(chainPath, []byte(chain), 0644); err != nil {
		t.Fatal(err)
	}

	// IDs only the stored v1 manifest mentions (e.g. v1's root directory
	// record) prove the closure: they appear in no root file.
	var v1Only []c4.ID
	chainIDs := idSet([]byte(chain))
	for id := range v1IDs {
		if !chainIDs[id] {
			v1Only = append(v1Only, id)
		}
	}
	if len(v1Only) == 0 {
		t.Fatal("test needs at least one ID reachable only via the stored base manifest")
	}

	_, stderr, code := runC4WithEnv(t, bin, env, "gc", "--force", chainPath)
	if code != 0 {
		t.Fatalf("gc --force exit %d: %s", code, stderr)
	}

	if !s.Has(baseID) {
		t.Fatal("gc deleted the referenced base manifest object")
	}
	for _, id := range v1Only {
		if !s.Has(id) {
			t.Fatalf("gc deleted %s, reachable through the stored base manifest", id)
		}
	}
	for id := range v2IDs {
		if !s.Has(id) {
			t.Fatalf("gc deleted v2 object %s", id)
		}
	}
	for id := range bIDs {
		if v1IDs[id] || v2IDs[id] {
			continue
		}
		if s.Has(id) {
			t.Fatalf("gc kept unreachable object %s", id)
		}
	}
}
