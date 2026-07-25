package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// entryID returns the trailing C4 ID of the listing line naming want.
func entryID(t *testing.T, listing, want string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[len(f)-2] != want {
			continue
		}
		id := f[len(f)-1]
		if strings.HasPrefix(id, "c4") && len(id) == 90 {
			return id
		}
	}
	t.Fatalf("no entry for %q in listing:\n%s", want, listing)
	return ""
}

// A file inside a scanned tree must be stored as its exact bytes, even
// when its content parses as c4m. Before the fix, storeFileEntry stored
// a canonicalized copy, so the original bytes were unrecoverable and
// the entry named content that did not match the file on disk.
func TestSnapshotStoresC4mFileRawBytes(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "data.txt"), []byte("payload\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A c4m description living inside the tree as an ordinary file —
	// the reuse-guide workflow puts one here routinely.
	guide, _, code := runC4(t, bin, "id", "-e", tree)
	if code != 0 {
		t.Fatalf("id -e exit %d", code)
	}
	guidePath := filepath.Join(tree, "guide.c4m")
	if err := os.WriteFile(guidePath, []byte(guide), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}

	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}
	listing, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("snapshot exit %d: %s", code, stderr)
	}

	id := entryID(t, listing, "guide.c4m")
	got, stderr, code := runC4WithEnv(t, bin, env, "cat", id)
	if code != 0 {
		t.Fatalf("guide.c4m entry names %s but it is not in the store: %s", id, stderr)
	}
	if got != string(want) {
		t.Errorf("stored bytes differ from the file on disk: disk %d bytes, store %d",
			len(want), len(got))
	}
}

// A folded (-S) snapshot names an ID-list object built from its members'
// IDs in range order. That object must be stored, or the listing
// references content that is not in the store.
func TestFoldedSnapshotStoresIDList(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "seq")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	const members = 5
	for i := 1; i <= members; i++ {
		name := filepath.Join(tree, "frame.000"+string(rune('0'+i))+".exr")
		body := "f" + string(rune('0'+i)) + "\n"
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}
	listing, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-S", tree)
	if code != 0 {
		t.Fatalf("snapshot exit %d: %s", code, stderr)
	}
	if !strings.Contains(listing, "[0001-0005]") {
		t.Fatalf("expected a folded entry, got:\n%s", listing)
	}

	// Every ID the listing names must resolve.
	var foldedID string
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		id := f[len(f)-1]
		if !strings.HasPrefix(id, "c4") || len(id) != 90 {
			continue
		}
		if _, stderr, code := runC4WithEnv(t, bin, env, "cat", id); code != 0 {
			t.Fatalf("listing names %s but it is not in the store: %s", id, stderr)
		}
		if strings.Contains(line, "[") {
			foldedID = id
		}
	}
	if foldedID == "" {
		t.Fatal("no folded entry ID found in listing")
	}

	// The ID list holds exactly the members, in order, each resolving
	// to its own content.
	idList, _, code := runC4WithEnv(t, bin, env, "cat", foldedID)
	if code != 0 {
		t.Fatalf("cat ID list exit %d", code)
	}
	if len(idList) != members*90 {
		t.Fatalf("ID list is %d bytes, want %d (%d members x 90)",
			len(idList), members*90, members)
	}
	for i := 0; i < members; i++ {
		got, _, code := runC4WithEnv(t, bin, env, "cat", idList[i*90:(i+1)*90])
		if code != 0 {
			t.Fatalf("member %d does not resolve", i+1)
		}
		if want := "f" + string(rune('0'+i+1)) + "\n"; got != want {
			t.Errorf("member %d = %q, want %q", i+1, got, want)
		}
	}
}
