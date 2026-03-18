package c4m

import (
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func testID(s string) c4.ID {
	return c4.Identify(strings.NewReader(s))
}

func makeTestManifest(entries ...*Entry) *Manifest {
	m := NewManifest()
	for _, e := range entries {
		m.AddEntry(e)
	}
	return m
}

func TestMerge_NoChanges(t *testing.T) {
	ts := time.Now().UTC()
	id := testID("file1")

	base := makeTestManifest(&Entry{Mode: 0644, Name: "file.txt", Size: 10, C4ID: id, Timestamp: ts})
	local := makeTestManifest(&Entry{Mode: 0644, Name: "file.txt", Size: 10, C4ID: id, Timestamp: ts})
	remote := makeTestManifest(&Entry{Mode: 0644, Name: "file.txt", Size: 10, C4ID: id, Timestamp: ts})

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
	if len(merged.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(merged.Entries))
	}
}

func TestMerge_LocalOnlyAdd(t *testing.T) {
	ts := time.Now().UTC()
	id1 := testID("shared")
	id2 := testID("local-new")

	base := makeTestManifest(&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts})
	local := makeTestManifest(
		&Entry{Mode: 0644, Name: "local.txt", Size: 20, C4ID: id2, Timestamp: ts},
		&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts},
	)
	remote := makeTestManifest(&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts})

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}

	names := entryNames(merged)
	if !names["local.txt"] {
		t.Error("missing local.txt")
	}
	if !names["shared.txt"] {
		t.Error("missing shared.txt")
	}
}

func TestMerge_RemoteOnlyAdd(t *testing.T) {
	ts := time.Now().UTC()
	id1 := testID("shared")
	id2 := testID("remote-new")

	base := makeTestManifest(&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts})
	local := makeTestManifest(&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts})
	remote := makeTestManifest(
		&Entry{Mode: 0644, Name: "remote.txt", Size: 20, C4ID: id2, Timestamp: ts},
		&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts},
	)

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}

	names := entryNames(merged)
	if !names["remote.txt"] {
		t.Error("missing remote.txt")
	}
	if !names["shared.txt"] {
		t.Error("missing shared.txt")
	}
}

func TestMerge_BothAddDifferentFiles(t *testing.T) {
	ts := time.Now().UTC()
	id1 := testID("shared")
	id2 := testID("local-new")
	id3 := testID("remote-new")

	base := makeTestManifest(&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts})
	local := makeTestManifest(
		&Entry{Mode: 0644, Name: "local.txt", Size: 20, C4ID: id2, Timestamp: ts},
		&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts},
	)
	remote := makeTestManifest(
		&Entry{Mode: 0644, Name: "remote.txt", Size: 30, C4ID: id3, Timestamp: ts},
		&Entry{Mode: 0644, Name: "shared.txt", Size: 10, C4ID: id1, Timestamp: ts},
	)

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}

	names := entryNames(merged)
	for _, want := range []string{"shared.txt", "local.txt", "remote.txt"} {
		if !names[want] {
			t.Errorf("missing %s in merged (has: %v)", want, names)
		}
	}
}

func TestMerge_ConflictSameFile(t *testing.T) {
	ts := time.Now().UTC()
	baseID := testID("original")
	localID := testID("local-edit")
	remoteID := testID("remote-edit")

	base := makeTestManifest(&Entry{Mode: 0644, Name: "doc.txt", Size: 10, C4ID: baseID, Timestamp: ts.Add(-2 * time.Hour)})
	local := makeTestManifest(&Entry{Mode: 0644, Name: "doc.txt", Size: 11, C4ID: localID, Timestamp: ts.Add(-30 * time.Minute)})
	remote := makeTestManifest(&Entry{Mode: 0644, Name: "doc.txt", Size: 12, C4ID: remoteID, Timestamp: ts.Add(-10 * time.Minute)})

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Path != "doc.txt" {
		t.Errorf("conflict path: got %q, want %q", conflicts[0].Path, "doc.txt")
	}

	names := entryNames(merged)
	if !names["doc.txt"] {
		t.Error("missing doc.txt (LWW winner)")
	}
	if !names["doc.txt.conflict"] {
		t.Error("missing doc.txt.conflict (preserved loser)")
	}

	// LWW: remote was newer, so doc.txt should have remoteID.
	for _, e := range merged.Entries {
		if e.Name == "doc.txt" && e.C4ID != remoteID {
			t.Errorf("doc.txt should have remote C4ID (newer), got %s", e.C4ID)
		}
		if e.Name == "doc.txt.conflict" && e.C4ID != localID {
			t.Errorf("doc.txt.conflict should have local C4ID, got %s", e.C4ID)
		}
	}
}

func TestMerge_LocalDelete(t *testing.T) {
	ts := time.Now().UTC()
	id := testID("gone")

	base := makeTestManifest(
		&Entry{Mode: 0644, Name: "keep.txt", Size: 10, C4ID: testID("keep"), Timestamp: ts},
		&Entry{Mode: 0644, Name: "gone.txt", Size: 10, C4ID: id, Timestamp: ts},
	)
	local := makeTestManifest(
		&Entry{Mode: 0644, Name: "keep.txt", Size: 10, C4ID: testID("keep"), Timestamp: ts},
	)
	remote := makeTestManifest(
		&Entry{Mode: 0644, Name: "keep.txt", Size: 10, C4ID: testID("keep"), Timestamp: ts},
		&Entry{Mode: 0644, Name: "gone.txt", Size: 10, C4ID: id, Timestamp: ts},
	)

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}

	names := entryNames(merged)
	if names["gone.txt"] {
		t.Error("gone.txt should be deleted (local deleted, remote unchanged)")
	}
	if !names["keep.txt"] {
		t.Error("keep.txt should still exist")
	}
}

func TestMerge_BothDeleteSameFile(t *testing.T) {
	ts := time.Now().UTC()
	base := makeTestManifest(
		&Entry{Mode: 0644, Name: "file.txt", Size: 10, C4ID: testID("x"), Timestamp: ts},
	)
	local := makeTestManifest()
	remote := makeTestManifest()

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
	if len(merged.Entries) != 0 {
		t.Errorf("expected empty merged, got %d entries", len(merged.Entries))
	}
}

func TestMerge_NilBase(t *testing.T) {
	ts := time.Now().UTC()
	id := testID("content")

	local := makeTestManifest(&Entry{Mode: 0644, Name: "file.txt", Size: 10, C4ID: id, Timestamp: ts})
	remote := makeTestManifest()

	merged, _, err := Merge(nil, local, remote)
	if err != nil {
		t.Fatal(err)
	}

	names := entryNames(merged)
	if !names["file.txt"] {
		t.Error("file.txt should be in merged (added by local, nil base)")
	}
}

func TestMerge_NestedPaths(t *testing.T) {
	ts := time.Now().UTC()
	id1 := testID("local-nested")
	id2 := testID("remote-nested")

	base := makeTestManifest(
		&Entry{Mode: 0755 | 1<<31, Name: "dir/", Size: -1, Depth: 0, Timestamp: ts},
	)
	local := makeTestManifest(
		&Entry{Mode: 0755 | 1<<31, Name: "dir/", Size: -1, Depth: 0, Timestamp: ts},
		&Entry{Mode: 0644, Name: "a.txt", Size: 10, C4ID: id1, Depth: 1, Timestamp: ts},
	)
	remote := makeTestManifest(
		&Entry{Mode: 0755 | 1<<31, Name: "dir/", Size: -1, Depth: 0, Timestamp: ts},
		&Entry{Mode: 0644, Name: "b.txt", Size: 10, C4ID: id2, Depth: 1, Timestamp: ts},
	)

	merged, conflicts, err := Merge(base, local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}

	// Should have dir/, dir/a.txt, dir/b.txt
	pathMap := EntryPaths(merged.Entries)
	for _, want := range []string{"dir/", "dir/a.txt", "dir/b.txt"} {
		if _, ok := pathMap[want]; !ok {
			t.Errorf("missing %q in merged", want)
		}
	}
}

func entryNames(m *Manifest) map[string]bool {
	names := make(map[string]bool)
	for _, e := range m.Entries {
		names[e.Name] = true
	}
	return names
}
