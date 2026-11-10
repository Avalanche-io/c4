package c4m

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func apfEntry(name, content string, depth int) *Entry {
	return &Entry{
		Name: name, Mode: 0644, Size: int64(len(content)), Depth: depth,
		Timestamp: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
		C4ID:      c4.Identify(strings.NewReader(content)),
	}
}

func apfManifest(entries ...*Entry) *Manifest {
	m := NewManifest()
	for _, e := range entries {
		cp := *e
		m.AddEntry(&cp)
	}
	return m
}

func apfPaths(m *Manifest) map[string]*Entry { return EntryPaths(m.Entries) }

// With target equal to base, ApplyPatchFrom is ApplyPatch, for every
// patch meaning: addition, modification, removal and deepening.
func TestApplyPatchFromEqualsApplyPatchOnItsOwnBase(t *testing.T) {
	base := apfManifest(
		apfEntry("a.txt", "A", 0),
		apfEntry("gone.txt", "G", 0),
		&Entry{Name: "d/", Mode: 0755 | os.ModeDir, Size: -1, Depth: 0},
		apfEntry("old.txt", "O", 1),
	)
	patch := apfManifest(
		apfEntry("a.txt", "A2", 0),                                      // modification
		apfEntry("gone.txt", "G", 0),                                    // identical, no children: removal
		apfEntry("new.txt", "N", 0),                                     // addition
		&Entry{Name: "d/", Mode: 0755 | os.ModeDir, Size: -1, Depth: 0}, // deepening
		apfEntry("deep.txt", "D", 1),
	)
	want := ApplyPatch(base, patch)
	got := ApplyPatchFrom(base, base, patch)
	if got.Canonical() != want.Canonical() {
		t.Fatalf("ApplyPatchFrom(base, base, p) must equal ApplyPatch(base, p):\n--- got\n%s--- want\n%s", got.Canonical(), want.Canonical())
	}
	paths := apfPaths(got)
	if paths["gone.txt"] != nil || paths["new.txt"] == nil || paths["d/deep.txt"] == nil || paths["d/old.txt"] == nil {
		t.Fatalf("meanings misapplied: %v", keysOf(paths))
	}
}

// The operator's case: two units add the same bytes at the same path.
// Applied directly, the second add meets an identical entry and reads as
// a removal; read against each patch's own base, both mean "written".
// Replaying the same fold is a no-op, not an undo.
func TestApplyPatchFromIsIdempotentForSameAdds(t *testing.T) {
	base := apfManifest(apfEntry("keep.txt", "K", 0))
	patch := apfManifest(apfEntry("out.txt", "same bytes", 0)) // diffed from base: an addition

	once := ApplyPatchFrom(base, base, patch)
	if apfPaths(once)["out.txt"] == nil {
		t.Fatal("first add must land")
	}
	// Direct application to the already-updated tree reads the add as a
	// removal: the bug.
	if apfPaths(ApplyPatch(once, patch))["out.txt"] != nil {
		t.Fatal("precondition: plain ApplyPatch on the wrong base must remove the path (that is the bug this fixes)")
	}
	twice := ApplyPatchFrom(once, base, patch)
	if apfPaths(twice)["out.txt"] == nil {
		t.Fatal("replaying the add against its own base must keep the path")
	}
	if twice.Canonical() != once.Canonical() {
		t.Fatalf("replay must be a no-op:\n%s\nvs\n%s", once.Canonical(), twice.Canonical())
	}
}

// A removal in the patch removes the path in the target whether or not
// the target's entry matches the base's; a path the target never had
// stays absent. A path the target holds differently is overwritten by a
// written entry: the patch wins, with no policy of its own.
func TestApplyPatchFromCarriesMeaningToAForeignTarget(t *testing.T) {
	base := apfManifest(apfEntry("gone.txt", "G", 0), apfEntry("edit.txt", "E1", 0))
	patch := apfManifest(apfEntry("gone.txt", "G", 0), apfEntry("edit.txt", "E2", 0)) // removal + modification
	target := apfManifest(
		apfEntry("gone.txt", "G-changed-elsewhere", 0), // differs from base: still removed
		apfEntry("edit.txt", "E-target", 0),            // differs from base: patch wins
		apfEntry("other.txt", "T", 0),                  // untouched
	)
	got := apfPaths(ApplyPatchFrom(target, base, patch))
	if got["gone.txt"] != nil {
		t.Fatal("a removal must remove the path in the target")
	}
	if e := got["edit.txt"]; e == nil || e.C4ID != c4.Identify(strings.NewReader("E2")) {
		t.Fatal("a written entry must win in the target")
	}
	if got["other.txt"] == nil {
		t.Fatal("paths the patch does not name are untouched")
	}
	// A removal of a path the target never held is a no-op.
	empty := apfPaths(ApplyPatchFrom(apfManifest(apfEntry("other.txt", "T", 0)), base, patch))
	if empty["gone.txt"] != nil || empty["other.txt"] == nil || empty["edit.txt"] == nil {
		t.Fatalf("foreign target: %v", keysOf(empty))
	}
}

func keysOf(m map[string]*Entry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
