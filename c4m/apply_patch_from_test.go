package c4m

import (
	"bytes"
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
	Normalize(want) // ApplyPatchFrom derives directory values; compare on the same footing
	got := ApplyPatchFrom(base, base, patch)
	if got.Canonical() != want.Canonical() {
		t.Fatalf("ApplyPatchFrom(base, base, p) must equal ApplyPatch(base, p) up to derived directory values:\n--- got\n%s--- want\n%s", got.Canonical(), want.Canonical())
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

// A deepened directory (its line restated with children) carried to a
// target that lacks the directory creates it and applies the children
// against the base's children: an added child lands, a child restated
// identically to the base's means removal and so does not appear.
func TestApplyPatchFromDeepensIntoATargetLackingTheDirectory(t *testing.T) {
	dir := &Entry{Name: "d/", Mode: 0755 | os.ModeDir, Size: -1, Depth: 0}
	base := apfManifest(dir, apfEntry("old.txt", "O", 1))
	patch := apfManifest(dir, apfEntry("old.txt", "O", 1), apfEntry("new.txt", "N", 1)) // deepen: remove old, add new
	target := apfManifest(apfEntry("other.txt", "T", 0))

	got := apfPaths(ApplyPatchFrom(target, base, patch))
	if got["d/"] == nil || !got["d/"].IsDir() {
		t.Fatalf("the deepened directory must be created in the target: %v", keysOf(got))
	}
	if got["d/new.txt"] == nil {
		t.Fatal("the added child must land under the created directory")
	}
	if got["d/old.txt"] != nil {
		t.Fatal("a child restated identically to the base's is a removal and must not appear")
	}
	if got["other.txt"] == nil {
		t.Fatal("unrelated target paths stay")
	}
}

// A directory line with children under it is context, not a write. A
// patch that restates d/ only to remove d/f, carried to a target that no
// longer has d/, leaves the target unchanged: the unit never made that
// directory, it only stopped having a file in it (found by the operator's
// oracle test, 2026-11-09).
func TestApplyPatchFromRemovalOnlyDeepeningDoesNotRecreateTheDirectory(t *testing.T) {
	dir := &Entry{Name: "d/", Mode: 0755 | os.ModeDir, Size: -1, Depth: 0}
	base := apfManifest(dir, apfEntry("f", "F", 1))
	patch := apfManifest(dir, apfEntry("f", "F", 1)) // d/ restated, f restated: remove d/f
	target := apfManifest(apfEntry("other.txt", "T", 0))

	got := ApplyPatchFrom(target, base, patch)
	if apfPaths(got)["d/"] != nil {
		t.Fatalf("a removal-only deepening must not recreate d/ in a target that lacks it:\n%s", got.Canonical())
	}
	if got.Canonical() != target.Canonical() {
		t.Fatalf("target must be unchanged:\n%s\nvs\n%s", got.Canonical(), target.Canonical())
	}
}

// A directory the target holds with other children keeps its own line;
// the patch's directory line (this unit's aggregate) is never copied onto
// it, and its size and ID are derived from the children it now lists.
func TestApplyPatchFromKeepsTheTargetsDirectoryLineAndDerivesAggregates(t *testing.T) {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	baseDir := &Entry{Name: "d/", Mode: 0755 | os.ModeDir, Size: -1, Timestamp: ts, Depth: 0}
	base := apfManifest(baseDir, apfEntry("f", "F", 1))
	// The unit's own d/ after adding g: a different line (another time).
	unitDir := &Entry{Name: "d/", Mode: 0755 | os.ModeDir, Size: -1, Timestamp: ts.Add(time.Hour), Depth: 0}
	patch := apfManifest(unitDir, apfEntry("g", "G", 1))
	// The target's d/ was changed by another unit (x added) and has its own time.
	targetDir := &Entry{Name: "d/", Mode: 0755 | os.ModeDir, Size: -1, Timestamp: ts.Add(2 * time.Hour), Depth: 0}
	target := apfManifest(targetDir, apfEntry("f", "F", 1), apfEntry("x", "X", 1))

	got := apfPaths(ApplyPatchFrom(target, base, patch))
	d := got["d/"]
	if d == nil || !d.Timestamp.Equal(ts.Add(2*time.Hour)) {
		t.Fatalf("the target's own directory line must be kept, got %v", d)
	}
	if got["d/f"] == nil || got["d/g"] == nil || got["d/x"] == nil {
		t.Fatalf("d/ must hold f, g and x: %v", keysOf(got))
	}
	// Size and ID are derived from f, g and x, not copied from the patch.
	children := []*Entry{got["d/f"], got["d/g"], got["d/x"]}
	_, wantID := ListingRecord(children)
	if d.C4ID != wantID || d.Size < 0 {
		t.Fatalf("directory aggregates must be derived from the listed children: size %d, id %s, want %s", d.Size, d.C4ID, wantID)
	}

	// Where the target's line is still the base's, the patch's line is the
	// same observation continued and replaces it (so target == base gives
	// ApplyPatch's answer).
	same := apfPaths(ApplyPatchFrom(base, base, patch))
	if sd := same["d/"]; sd == nil || !sd.Timestamp.Equal(ts.Add(time.Hour)) {
		t.Fatalf("on its own base the patch's directory line applies, got %v", sd)
	}
}

// The operator's cross-check (seed 9): the patch removes the last child
// of a directory whose line in the target differs from the base's. The
// kept target line carried the ID of a directory still holding the
// child; with no children left, Normalize could not tell it from a
// directory known only by its ID, so the removal was undone at the ID.
// A directory the walk entered is re-derived from what remains: here the
// empty directory.
func TestApplyPatchFromRederivesAnEmptiedDirectory(t *testing.T) {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	dir := func(size int64, id c4.ID) *Entry {
		return &Entry{Name: "weights/", Mode: 0, Size: size, C4ID: id, Timestamp: ts, Depth: 0}
	}
	model := apfEntry("model.bin", "weights", 1) // mode 0644
	modelX := apfEntry("model.bin", "weights", 1)
	modelX.Mode = 0755 // the other unit made it executable

	base := apfManifest(dir(-1, c4.ID{}), model)
	Normalize(base)
	empty := c4.Identify(bytes.NewReader(nil))
	patch := apfManifest(dir(0, empty), model) // weights/ now empty; model.bin restated: removal
	target := apfManifest(dir(-1, c4.ID{}), modelX)
	Normalize(target) // target's weights/ line differs from base's (its child differs)

	got := ApplyPatchFrom(target, base, patch)
	paths := apfPaths(got)
	if paths["weights/model.bin"] != nil {
		t.Fatal("model.bin must be removed")
	}
	w := paths["weights/"]
	if w == nil {
		t.Fatal("weights/ stays: the unit left it empty, it did not remove it")
	}
	if w.Size != 0 || w.C4ID != empty {
		t.Fatalf("an emptied directory is the empty directory, got size %d id %s", w.Size, w.C4ID)
	}
	if w.C4ID == apfPaths(target)["weights/"].C4ID {
		t.Fatal("the target's stale directory ID must not survive the removal")
	}
}
