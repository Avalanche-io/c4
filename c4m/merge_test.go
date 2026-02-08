package c4m

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestMapGetter_Get(t *testing.T) {
	m := NewManifest()
	m.AddEntry(&Entry{Name: "test.txt", Size: 100})
	id := m.ComputeC4ID()

	source := MapGetter{id: m}

	// Found
	got, err := source.Get(id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != m {
		t.Error("Get() returned different manifest")
	}

	// Not found
	fakeID := c4.Identify(strings.NewReader("fake"))
	_, err = source.Get(fakeID)
	if err == nil {
		t.Error("Get() expected error for missing ID")
	}
}

func TestManifest_Merge_Simple(t *testing.T) {
	// Create a simple manifest with no base
	m := NewBuilder().
		AddFile("file1.txt", WithSize(100)).
		AddFile("file2.txt", WithSize(200)).
		MustBuild()

	source := MapGetter{}

	merged, err := m.Merge(source)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	if len(merged.Entries) != 2 {
		t.Errorf("Merge() got %d entries, want 2", len(merged.Entries))
	}
}

func TestManifest_Merge_WithBase(t *testing.T) {
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Step 1: Base manifest
	step1 := NewBuilder().
		AddFile("readme.txt", WithSize(100), WithTimestamp(ts)).
		AddFile("config.json", WithSize(250), WithTimestamp(ts)).
		MustBuild()
	step1ID := step1.ComputeC4ID()

	// Step 2: Extends step1
	step2 := NewBuilder().
		WithBaseID(step1ID).
		AddFile("main.go", WithSize(500), WithTimestamp(ts)).
		MustBuild()

	source := MapGetter{step1ID: step1}

	merged, err := step2.Merge(source)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	// Should have all 3 files
	if len(merged.Entries) != 3 {
		t.Errorf("Merge() got %d entries, want 3", len(merged.Entries))
	}

	// Check no base reference in merged
	if !merged.Base.IsNil() {
		t.Error("Merge() result should have no @base")
	}

	// Verify all files present
	names := make(map[string]bool)
	for _, e := range merged.Entries {
		names[e.Name] = true
	}
	for _, want := range []string{"readme.txt", "config.json", "main.go"} {
		if !names[want] {
			t.Errorf("Merge() missing entry %q", want)
		}
	}
}

func TestManifest_Merge_WithRemovals(t *testing.T) {
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Base manifest with 3 files
	base := NewBuilder().
		AddFile("keep.txt", WithSize(100), WithTimestamp(ts)).
		AddFile("remove.txt", WithSize(200), WithTimestamp(ts)).
		AddFile("also_keep.txt", WithSize(300), WithTimestamp(ts)).
		MustBuild()
	baseID := base.ComputeC4ID()

	// Layer that removes one file
	layer := NewBuilder().
		WithBase(base).
		Remove("remove.txt").
		AddFile("new.txt", WithSize(400), WithTimestamp(ts)).
		MustBuild()

	source := MapGetter{baseID: base}

	merged, err := layer.Merge(source)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	// Should have 3 files (keep, also_keep, new) - not remove.txt
	if len(merged.Entries) != 3 {
		t.Errorf("Merge() got %d entries, want 3", len(merged.Entries))
	}

	// Verify remove.txt is gone
	for _, e := range merged.Entries {
		if e.Name == "remove.txt" {
			t.Error("Merge() should have removed remove.txt")
		}
	}
}

func TestManifest_Merge_DeepChain(t *testing.T) {
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Build a 3-level chain
	step1 := NewBuilder().
		AddFile("file1.txt", WithSize(100), WithTimestamp(ts)).
		MustBuild()
	step1ID := step1.ComputeC4ID()

	step2 := NewBuilder().
		WithBaseID(step1ID).
		AddFile("file2.txt", WithSize(200), WithTimestamp(ts)).
		MustBuild()
	step2ID := step2.ComputeC4ID()

	step3 := NewBuilder().
		WithBaseID(step2ID).
		AddFile("file3.txt", WithSize(300), WithTimestamp(ts)).
		MustBuild()

	source := MapGetter{
		step1ID: step1,
		step2ID: step2,
	}

	merged, err := step3.Merge(source)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	// Should have all 3 files
	if len(merged.Entries) != 3 {
		t.Errorf("Merge() got %d entries, want 3", len(merged.Entries))
	}
}

func TestManifest_Merge_MissingBase(t *testing.T) {
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	fakeBaseID := c4.Identify(strings.NewReader("missing base"))

	m := NewBuilder().
		WithBaseID(fakeBaseID).
		AddFile("file.txt", WithSize(100), WithTimestamp(ts)).
		MustBuild()

	source := MapGetter{} // Empty - base not available

	_, err := m.Merge(source)
	if err == nil {
		t.Error("Merge() expected error for missing base")
	}
}

func TestManifest_Removals(t *testing.T) {
	base := NewBuilder().
		AddFile("file1.txt", WithSize(100)).
		AddFile("file2.txt", WithSize(200)).
		MustBuild()

	layer, _ := NewBuilder().
		WithBase(base).
		Remove("file1.txt").
		Remove("file2.txt").
		Build()

	removals := layer.Removals()
	if len(removals) != 2 {
		t.Errorf("Removals() got %d, want 2", len(removals))
	}

	// Check both files are in removals
	found := make(map[string]bool)
	for _, r := range removals {
		found[r] = true
	}
	if !found["file1.txt"] || !found["file2.txt"] {
		t.Error("Removals() missing expected paths")
	}
}

func TestEntry_InRemoveLayer(t *testing.T) {
	base := NewBuilder().
		AddFile("file.txt", WithSize(100)).
		MustBuild()

	layer, _ := NewBuilder().
		WithBase(base).
		Remove("file.txt").
		Build()

	// Find the removal entry
	var removeEntry *Entry
	for _, e := range layer.Entries {
		if e.Name == "file.txt" {
			removeEntry = e
			break
		}
	}

	if removeEntry == nil {
		t.Fatal("Could not find removal entry")
	}

	if !removeEntry.InRemoveLayer() {
		t.Error("InRemoveLayer() should return true for removal entry")
	}

	// Regular entry should return false
	regularEntry := &Entry{Name: "regular.txt"}
	if regularEntry.InRemoveLayer() {
		t.Error("InRemoveLayer() should return false for regular entry")
	}
}

func TestManifest_Merge_PreservesDataBlocks(t *testing.T) {
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create a manifest with a data block
	metadataJSON := []byte(`{"key": "value"}`)
	metadataID := c4.Identify(strings.NewReader(string(metadataJSON)))

	m := NewBuilder().
		AddFile(".meta.json", WithC4ID(metadataID), WithSize(int64(len(metadataJSON))), WithTimestamp(ts)).
		MustBuild()

	m.AddDataBlock(&DataBlock{
		ID:      metadataID,
		Content: metadataJSON,
	})

	source := MapGetter{}

	merged, err := m.Merge(source)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	// Check data block is preserved
	if len(merged.DataBlocks) != 1 {
		t.Errorf("Merge() got %d data blocks, want 1", len(merged.DataBlocks))
	}

	block := merged.GetDataBlock(metadataID)
	if block == nil {
		t.Error("Merge() did not preserve data block")
	}
}

// mockSource implements store.Source for testing storeAdapter.
type mockSource struct {
	data map[string][]byte // c4 ID string -> manifest bytes
}

func (m *mockSource) Open(id c4.ID) (io.ReadCloser, error) {
	data, ok := m.data[id.String()]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func TestFromStore_Success(t *testing.T) {
	// Encode a valid manifest to bytes
	manifest := NewBuilder().
		AddFile("hello.txt", WithSize(5)).
		MustBuild()

	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(manifest); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	encoded := buf.Bytes()

	// Compute the ID of the encoded manifest content
	manifestID := c4.Identify(bytes.NewReader(encoded))

	src := &mockSource{data: map[string][]byte{
		manifestID.String(): encoded,
	}}

	getter := FromStore(src)
	got, err := getter.Get(manifestID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("Get() entries = %d, want 1", len(got.Entries))
	}
	if got.Entries[0].Name != "hello.txt" {
		t.Errorf("Get() entry name = %q, want %q", got.Entries[0].Name, "hello.txt")
	}
}

func TestFromStore_OpenError(t *testing.T) {
	src := &mockSource{data: map[string][]byte{}} // empty — all lookups fail

	getter := FromStore(src)
	missingID := c4.Identify(strings.NewReader("missing"))
	_, err := getter.Get(missingID)
	if err == nil {
		t.Fatal("Get() expected error for missing ID")
	}
	if !strings.Contains(err.Error(), "open manifest") {
		t.Errorf("Get() error = %q, want containing 'open manifest'", err.Error())
	}
}

func TestFromStore_DecodeError(t *testing.T) {
	// Store returns content that is not a valid manifest
	garbage := []byte("this is not a c4m manifest")
	garbageID := c4.Identify(bytes.NewReader(garbage))

	src := &mockSource{data: map[string][]byte{
		garbageID.String(): garbage,
	}}

	getter := FromStore(src)
	_, err := getter.Get(garbageID)
	if err == nil {
		t.Fatal("Get() expected error for invalid manifest content")
	}
	if !strings.Contains(err.Error(), "decode manifest") {
		t.Errorf("Get() error = %q, want containing 'decode manifest'", err.Error())
	}
}
