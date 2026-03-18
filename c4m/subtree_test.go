package c4m

import (
	"testing"
	"time"
)

func TestExtractSubtree_RootLevel(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddFile("readme.txt", WithC4ID(testID("readme")), WithTimestamp(ts)).
		AddDir("shared", WithFlowSync("backup:collab/"), WithTimestamp(ts)).
			AddFile("doc.txt", WithC4ID(testID("doc")), WithTimestamp(ts)).
			AddFile("img.png", WithC4ID(testID("img")), WithTimestamp(ts)).
		End().
		AddDir("src", WithTimestamp(ts)).
			AddFile("main.go", WithC4ID(testID("main")), WithTimestamp(ts)).
		End().
		MustBuild()

	sub, err := m.ExtractSubtree("shared/")
	if err != nil {
		t.Fatalf("ExtractSubtree: %v", err)
	}

	if len(sub.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(sub.Entries))
	}

	for _, e := range sub.Entries {
		if e.Depth != 0 {
			t.Errorf("entry %s: expected depth 0, got %d", e.Name, e.Depth)
		}
	}

	names := make(map[string]bool)
	for _, e := range sub.Entries {
		names[e.Name] = true
	}
	if !names["doc.txt"] || !names["img.png"] {
		t.Errorf("expected doc.txt and img.png, got %v", names)
	}
}

func TestExtractSubtree_Nested(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddDir("project", WithTimestamp(ts)).
			AddDir("src", WithTimestamp(ts)).
				AddDir("shared", WithFlowSync("backup:things/"), WithTimestamp(ts)).
					AddFile("a.txt", WithC4ID(testID("a")), WithTimestamp(ts)).
					AddFile("b.txt", WithC4ID(testID("b")), WithTimestamp(ts)).
				EndDir().
			EndDir().
		End().
		MustBuild()

	sub, err := m.ExtractSubtree("project/src/shared/")
	if err != nil {
		t.Fatalf("ExtractSubtree: %v", err)
	}

	if len(sub.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(sub.Entries))
	}

	for _, e := range sub.Entries {
		if e.Depth != 0 {
			t.Errorf("entry %s: depth %d, want 0", e.Name, e.Depth)
		}
	}
}

func TestExtractSubtree_Empty(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddDir("empty", WithFlowSync("backup:stuff/"), WithTimestamp(ts)).
		End().
		MustBuild()

	sub, err := m.ExtractSubtree("empty/")
	if err != nil {
		t.Fatalf("ExtractSubtree: %v", err)
	}

	if len(sub.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(sub.Entries))
	}
}

func TestExtractSubtree_NotFound(t *testing.T) {
	m := NewManifest()
	_, err := m.ExtractSubtree("nonexistent/")
	if err == nil {
		t.Fatal("expected error for nonexistent entry")
	}
}

func TestExtractSubtree_NotDir(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddFile("file.txt", WithC4ID(testID("file")), WithTimestamp(ts)).
		MustBuild()

	_, err := m.ExtractSubtree("file.txt")
	if err == nil {
		t.Fatal("expected error for non-directory entry")
	}
}

func TestExtractSubtree_DeepNesting(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddDir("shared", WithFlowSync("backup:stuff/"), WithTimestamp(ts)).
			AddFile("top.txt", WithC4ID(testID("top")), WithTimestamp(ts)).
			AddDir("sub", WithTimestamp(ts)).
				AddFile("deep.txt", WithC4ID(testID("deep")), WithTimestamp(ts)).
			EndDir().
		End().
		MustBuild()

	sub, err := m.ExtractSubtree("shared/")
	if err != nil {
		t.Fatalf("ExtractSubtree: %v", err)
	}

	// Should have: top.txt (depth 0), sub/ (depth 0), deep.txt (depth 1)
	if len(sub.Entries) != 3 {
		for _, e := range sub.Entries {
			t.Logf("  %s depth=%d", e.Name, e.Depth)
		}
		t.Fatalf("expected 3 entries, got %d", len(sub.Entries))
	}

	for _, e := range sub.Entries {
		switch e.Name {
		case "top.txt":
			if e.Depth != 0 {
				t.Errorf("top.txt: depth %d, want 0", e.Depth)
			}
		case "sub/":
			if e.Depth != 0 {
				t.Errorf("sub/: depth %d, want 0", e.Depth)
			}
		case "deep.txt":
			if e.Depth != 1 {
				t.Errorf("deep.txt: depth %d, want 1", e.Depth)
			}
		default:
			t.Errorf("unexpected entry: %s", e.Name)
		}
	}
}

func TestInjectSubtree_Replace(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddFile("readme.txt", WithC4ID(testID("readme")), WithTimestamp(ts)).
		AddDir("shared", WithFlowSync("backup:collab/"), WithTimestamp(ts)).
			AddFile("old.txt", WithC4ID(testID("old")), WithTimestamp(ts)).
		End().
		AddDir("src", WithTimestamp(ts)).
			AddFile("main.go", WithC4ID(testID("main")), WithTimestamp(ts)).
		End().
		MustBuild()

	sub := NewBuilder().
		AddFile("new1.txt", WithC4ID(testID("new1")), WithTimestamp(ts)).
		AddFile("new2.txt", WithC4ID(testID("new2")), WithTimestamp(ts)).
		MustBuild()

	result, err := m.InjectSubtree("shared/", sub)
	if err != nil {
		t.Fatalf("InjectSubtree: %v", err)
	}

	// Original: readme.txt, shared/, old.txt, src/, main.go = 5 entries
	// After: readme.txt, shared/, new1.txt, new2.txt, src/, main.go = 6 entries
	if len(result.Entries) != 6 {
		for _, e := range result.Entries {
			t.Logf("  %s (depth %d)", e.Name, e.Depth)
		}
		t.Fatalf("expected 6 entries, got %d", len(result.Entries))
	}

	// Verify shared/ entry is preserved with flow link.
	var shared *Entry
	for _, e := range result.Entries {
		if e.Name == "shared/" {
			shared = e
			break
		}
	}
	if shared == nil {
		t.Fatal("shared/ entry not found in result")
	}
	if shared.FlowDirection != FlowBidirectional {
		t.Errorf("shared/ flow direction: %v, want bidirectional", shared.FlowDirection)
	}
	if shared.FlowTarget != "backup:collab/" {
		t.Errorf("shared/ flow target: %q, want %q", shared.FlowTarget, "backup:collab/")
	}

	// Verify new children are present at correct depth.
	found := make(map[string]int)
	for _, e := range result.Entries {
		found[e.Name] = e.Depth
	}
	if d, ok := found["new1.txt"]; !ok || d != 1 {
		t.Errorf("new1.txt: depth=%d ok=%v, want depth=1", d, ok)
	}
	if d, ok := found["new2.txt"]; !ok || d != 1 {
		t.Errorf("new2.txt: depth=%d ok=%v, want depth=1", d, ok)
	}
	if _, ok := found["old.txt"]; ok {
		t.Error("old.txt should have been removed")
	}
	if _, ok := found["readme.txt"]; !ok {
		t.Error("readme.txt should be preserved")
	}
	if _, ok := found["main.go"]; !ok {
		t.Error("main.go should be preserved")
	}
}

func TestInjectSubtree_Empty(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddDir("shared", WithFlowSync("backup:collab/"), WithTimestamp(ts)).
			AddFile("doc.txt", WithC4ID(testID("doc")), WithTimestamp(ts)).
		End().
		MustBuild()

	empty := NewManifest()
	result, err := m.InjectSubtree("shared/", empty)
	if err != nil {
		t.Fatalf("InjectSubtree: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].Name != "shared/" {
		t.Errorf("expected shared/, got %s", result.Entries[0].Name)
	}
}

func TestInjectSubtree_PreservesFlowLink(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddDir("data", WithFlowOutbound("backup:archive/"), WithTimestamp(ts)).
			AddFile("old.txt", WithC4ID(testID("old")), WithTimestamp(ts)).
		End().
		MustBuild()

	sub := NewBuilder().
		AddFile("new.txt", WithC4ID(testID("new")), WithTimestamp(ts)).
		MustBuild()

	result, err := m.InjectSubtree("data/", sub)
	if err != nil {
		t.Fatalf("InjectSubtree: %v", err)
	}

	var data *Entry
	for _, e := range result.Entries {
		if e.Name == "data/" {
			data = e
			break
		}
	}
	if data == nil {
		t.Fatal("data/ entry not found")
	}
	if data.FlowDirection != FlowOutbound {
		t.Errorf("flow direction: %v, want outbound", data.FlowDirection)
	}
	if data.FlowTarget != "backup:archive/" {
		t.Errorf("flow target: %q, want %q", data.FlowTarget, "backup:archive/")
	}
}

func TestExtractInject_RoundTrip(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddFile("readme.txt", WithC4ID(testID("readme")), WithTimestamp(ts)).
		AddDir("shared", WithFlowSync("backup:collab/"), WithTimestamp(ts)).
			AddFile("doc.txt", WithC4ID(testID("doc")), WithTimestamp(ts)).
			AddFile("img.png", WithC4ID(testID("img")), WithTimestamp(ts)).
		End().
		AddDir("src", WithTimestamp(ts)).
			AddFile("main.go", WithC4ID(testID("main")), WithTimestamp(ts)).
		End().
		MustBuild()

	sub, err := m.ExtractSubtree("shared/")
	if err != nil {
		t.Fatalf("ExtractSubtree: %v", err)
	}

	result, err := m.InjectSubtree("shared/", sub)
	if err != nil {
		t.Fatalf("InjectSubtree: %v", err)
	}

	if len(result.Entries) != len(m.Entries) {
		t.Fatalf("entry count: got %d, want %d", len(result.Entries), len(m.Entries))
	}

	for i, e := range result.Entries {
		orig := m.Entries[i]
		if e.Name != orig.Name || e.Depth != orig.Depth || e.C4ID != orig.C4ID {
			t.Errorf("entry %d: got {%s d=%d id=%s}, want {%s d=%d id=%s}",
				i, e.Name, e.Depth, e.C4ID, orig.Name, orig.Depth, orig.C4ID)
		}
	}
}

func TestEntryTreePath(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddDir("project", WithTimestamp(ts)).
			AddDir("src", WithTimestamp(ts)).
				AddDir("shared", WithTimestamp(ts)).
					AddFile("file.txt", WithC4ID(testID("f")), WithTimestamp(ts)).
				EndDir().
			EndDir().
		End().
		MustBuild()

	tests := []struct {
		name string
		want string
	}{
		{"project/", "project/"},
		{"src/", "project/src/"},
		{"shared/", "project/src/shared/"},
		{"file.txt", "project/src/shared/file.txt"},
	}

	for _, tt := range tests {
		e := m.GetEntry(tt.name)
		if e == nil {
			t.Errorf("GetEntry(%q) returned nil", tt.name)
			continue
		}
		got := m.EntryTreePath(e)
		if got != tt.want {
			t.Errorf("EntryTreePath(%q): got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSplitTreePath(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"shared/", []string{"shared/"}},
		{"project/src/shared/", []string{"project/", "src/", "shared/"}},
		{"shared/doc.txt", []string{"shared/", "doc.txt"}},
		{"file.txt", []string{"file.txt"}},
	}

	for _, tt := range tests {
		got := splitTreePath(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitTreePath(%q): got %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitTreePath(%q)[%d]: got %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
