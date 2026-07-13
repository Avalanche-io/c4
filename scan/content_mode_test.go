package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// buildContentTree writes a small nested tree. Modes and mtimes are
// then perturbed per test to simulate a second checkout on a different
// machine (different umask, different clock).
func buildContentTree(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"a.txt":         "alpha",
		"sub/b.txt":     "bravo",
		"sub/deep/c.md": "charlie",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestContentModeMachineIndependent is the T3 regression test: two
// trees with byte-identical content but different mtimes and modes
// must produce the same manifest identity in content mode — and
// different identities in full mode (full-fidelity is a different
// claim by design).
func TestContentModeMachineIndependent(t *testing.T) {
	treeA := t.TempDir()
	treeB := t.TempDir()
	buildContentTree(t, treeA)
	buildContentTree(t, treeB)

	// Perturb tree B: different mtimes everywhere, different perms on
	// one file (a umask difference; exec-bit differences are also
	// invisible to content identity per the full-null decision).
	past := time.Date(2010, 3, 4, 5, 6, 7, 0, time.UTC)
	err := filepath.Walk(treeB, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, past, past)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(treeB, "a.txt"), 0755); err != nil {
		t.Fatal(err)
	}

	scanID := func(root string, mode ScanMode) string {
		m, err := Dir(root, WithMode(mode))
		if err != nil {
			t.Fatal(err)
		}
		return m.ComputeC4ID().String()
	}

	contentA := scanID(treeA, ModeContent)
	contentB := scanID(treeB, ModeContent)
	if contentA != contentB {
		t.Fatalf("content IDs differ for byte-identical trees:\nA %s\nB %s", contentA, contentB)
	}

	fullA := scanID(treeA, ModeFull)
	fullB := scanID(treeB, ModeFull)
	if fullA == fullB {
		t.Fatal("full-fidelity IDs should differ when mtimes/modes differ — that is the claim full mode makes")
	}

	// A content ID and a full ID of the same tree are different
	// projections and must not collide (like-mode comparison rule).
	if contentA == fullA {
		t.Fatal("content and full projections unexpectedly equal")
	}
}

// TestContentModeEntriesAreNulled pins the field treatment: every
// entry in a content-mode manifest has null mode and null timestamp,
// real sizes, and (for files) real IDs.
func TestContentModeEntriesAreNulled(t *testing.T) {
	tree := t.TempDir()
	buildContentTree(t, tree)

	m, err := Dir(tree, WithMode(ModeContent))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) == 0 {
		t.Fatal("no entries")
	}
	for _, e := range m.Entries {
		if e.Mode != 0 {
			t.Fatalf("entry %s: mode not null: %v", e.Name, e.Mode)
		}
		if !e.Timestamp.Equal(c4m.NullTimestamp()) {
			t.Fatalf("entry %s: timestamp not null: %v", e.Name, e.Timestamp)
		}
		if !e.IsDir() && e.C4ID.IsNil() {
			t.Fatalf("file entry %s: missing C4 ID", e.Name)
		}
		if !e.IsDir() && e.Size < 0 {
			t.Fatalf("file entry %s: size should be real, got %d", e.Name, e.Size)
		}
	}
}

// TestContentModeDirIDMatchesFreshScan pins the merkle property in
// content mode: a directory entry's content ID equals the content ID a
// fresh content-mode scan rooted at that directory produces.
func TestContentModeDirIDMatchesFreshScan(t *testing.T) {
	tree := t.TempDir()
	buildContentTree(t, tree)

	m, err := Dir(tree, WithMode(ModeContent))
	if err != nil {
		t.Fatal(err)
	}
	var subID string
	for _, e := range m.Entries {
		if e.IsDir() && (e.Name == "sub" || e.Name == "sub/") && e.Depth == 0 {
			subID = e.C4ID.String()
		}
	}
	if subID == "" {
		t.Fatal("sub directory entry not found or has no ID")
	}

	fresh, err := Dir(filepath.Join(tree, "sub"), WithMode(ModeContent))
	if err != nil {
		t.Fatal(err)
	}
	freshID := fresh.ComputeC4ID().String()
	if subID != freshID {
		t.Fatalf("content dir ID != fresh scan: %s != %s", subID, freshID)
	}
}
