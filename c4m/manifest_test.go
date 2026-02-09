package c4m

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestNewManifest(t *testing.T) {
	m := NewManifest()
	if m == nil {
		t.Fatal("NewManifest() returned nil")
	}
	if m.Version != "1.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.0")
	}
	if len(m.Entries) != 0 {
		t.Errorf("Entries length = %d, want 0", len(m.Entries))
	}
}

func TestManifestAddEntry(t *testing.T) {
	m := NewManifest()
	e1 := &Entry{Name: "file1.txt"}
	e2 := &Entry{Name: "file2.txt"}
	
	m.AddEntry(e1)
	if len(m.Entries) != 1 {
		t.Errorf("After first add: length = %d, want 1", len(m.Entries))
	}
	
	m.AddEntry(e2)
	if len(m.Entries) != 2 {
		t.Errorf("After second add: length = %d, want 2", len(m.Entries))
	}
	
	if m.Entries[0] != e1 || m.Entries[1] != e2 {
		t.Error("Entries not added in correct order")
	}
}

func TestManifestSort(t *testing.T) {
	m := NewManifest()
	
	// Add entries in wrong order
	m.AddEntry(&Entry{Name: "file10.txt"})
	m.AddEntry(&Entry{Name: "file2.txt"})
	m.AddEntry(&Entry{Name: "file1.txt"})
	m.AddEntry(&Entry{Name: "dir1/", Mode: os.ModeDir})
	
	m.SortEntries()

	// Check natural sort order - files before directories at same depth
	expected := []string{"file1.txt", "file2.txt", "file10.txt", "dir1/"}
	for i, e := range m.Entries {
		if e.Name != expected[i] {
			t.Errorf("Entry[%d] = %q, want %q", i, e.Name, expected[i])
		}
	}
}

func TestManifestEncode(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	testID, _ := c4.Parse("c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")
	dataID, _ := c4.Parse("c42j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")

	tests := []struct {
		name     string
		manifest *Manifest
		want     string
	}{
		{
			name: "basic manifest",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      100,
						Name:      "file1.txt",
						C4ID:      testID,
						Depth:     0,
					},
				},
			},
			want: "@c4m 1.0\n-rw-r--r-- 2024-01-15T10:30:00Z 100 file1.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB\n",
		},
		{
			name: "manifest with metadata",
			manifest: &Manifest{
				Version: "1.0",
				Data:    dataID,
				Entries: []*Entry{},
			},
			want: "@c4m 1.0\n@data c42j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB\n",
		},
		{
			name: "manifest with base",
			manifest: &Manifest{
				Version: "1.0",
				Base:    testID,
				Entries: []*Entry{},
			},
			want: "@c4m 1.0\n@base c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := NewEncoder(&buf).Encode(tt.manifest)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("Encode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManifestCanonical(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	fileID, _ := c4.Parse("c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")
	dirID, _ := c4.Parse("c42j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")
	
	tests := []struct {
		name     string
		manifest *Manifest
		want     string
	}{
		{
			name: "single level manifest",
			manifest: &Manifest{
				Entries: []*Entry{
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      100,
						Name:      "file1.txt",
						C4ID:      fileID,
						Depth:     0,
					},
					{
						Mode:      os.ModeDir | 0755,
						Timestamp: testTime,
						Size:      4096,
						Name:      "dir1/",
						C4ID:      dirID,
						Depth:     0,
					},
				},
			},
			// Files before directories in canonical form
			want: "-rw-r--r-- 2024-01-15T10:30:00Z 100 file1.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB\n" +
				"drwxr-xr-x 2024-01-15T10:30:00Z 4096 dir1/ c42j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB\n",
		},
		{
			name: "multi-level manifest (only depth 0 included)",
			manifest: &Manifest{
				Entries: []*Entry{
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      100,
						Name:      "file1.txt",
						C4ID:      fileID,
						Depth:     0,
					},
					{
						Mode:      os.ModeDir | 0755,
						Timestamp: testTime,
						Size:      4096,
						Name:      "dir1/",
						C4ID:      dirID,
						Depth:     0,
					},
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      200,
						Name:      "dir1/file2.txt",
						C4ID:      fileID,
						Depth:     1, // Should be excluded
					},
				},
			},
			// Only depth 0 entries
			want: "-rw-r--r-- 2024-01-15T10:30:00Z 100 file1.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB\n" +
				"drwxr-xr-x 2024-01-15T10:30:00Z 4096 dir1/ c42j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB\n",
		},
		{
			name: "empty manifest",
			manifest: &Manifest{
				Entries: []*Entry{},
			},
			want: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.Canonical()
			if got != tt.want {
				t.Errorf("Canonical() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManifestComputeC4ID(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	fileID, _ := c4.Parse("c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")
	
	m := &Manifest{
		Entries: []*Entry{
			{
				Mode:      0644,
				Timestamp: testTime,
				Size:      100,
				Name:      "file1.txt",
				C4ID:      fileID,
				Depth:     0,
			},
		},
	}
	
	id := m.ComputeC4ID()
	if id.IsNil() {
		t.Error("ComputeC4ID() returned nil ID")
	}
	
	// Compute again to ensure consistency
	id2 := m.ComputeC4ID()
	if id.String() != id2.String() {
		t.Errorf("ComputeC4ID() not consistent: %v != %v", id, id2)
	}
	
	// The ID should be the C4 of the canonical form
	canonical := m.Canonical()
	expectedID := c4.Identify(strings.NewReader(canonical))
	if id.String() != expectedID.String() {
		t.Errorf("ComputeC4ID() = %v, want %v (from canonical)", id, expectedID)
	}
}

func TestManifestGetEntry(t *testing.T) {
	m := NewManifest()
	e1 := &Entry{Name: "file1.txt"}
	e2 := &Entry{Name: "dir1/file2.txt"}
	m.AddEntry(e1)
	m.AddEntry(e2)
	
	tests := []struct {
		path string
		want *Entry
	}{
		{"file1.txt", e1},
		{"dir1/file2.txt", e2},
		{"nonexistent.txt", nil},
	}
	
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := m.GetEntry(tt.path)
			if got != tt.want {
				t.Errorf("GetEntry(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestManifestGetEntriesAtDepth(t *testing.T) {
	m := NewManifest()
	e0a := &Entry{Name: "file1.txt", Depth: 0}
	e0b := &Entry{Name: "file2.txt", Depth: 0}
	e1a := &Entry{Name: "dir/file3.txt", Depth: 1}
	e1b := &Entry{Name: "dir/file4.txt", Depth: 1}
	e2 := &Entry{Name: "dir/sub/file5.txt", Depth: 2}
	
	m.AddEntry(e0a)
	m.AddEntry(e0b)
	m.AddEntry(e1a)
	m.AddEntry(e1b)
	m.AddEntry(e2)
	
	tests := []struct {
		depth int
		want  int
	}{
		{0, 2},
		{1, 2},
		{2, 1},
		{3, 0},
	}
	
	for _, tt := range tests {
		t.Run(string(rune(tt.depth)), func(t *testing.T) {
			entries := m.GetEntriesAtDepth(tt.depth)
			if len(entries) != tt.want {
				t.Errorf("GetEntriesAtDepth(%d) returned %d entries, want %d", tt.depth, len(entries), tt.want)
			}
		})
	}
}

func TestEncodeLayer(t *testing.T) {
	tests := []struct {
		name    string
		layer   *Layer
		wantOut []string
	}{
		{
			name: "add layer",
			layer: &Layer{
				Type: LayerTypeAdd,
			},
			wantOut: []string{"@layer"},
		},
		{
			name: "remove layer",
			layer: &Layer{
				Type: LayerTypeRemove,
			},
			wantOut: []string{"@remove"},
		},
		{
			name: "layer with by",
			layer: &Layer{
				Type: LayerTypeAdd,
				By:   "user@example.com",
			},
			wantOut: []string{"@layer", "@by user@example.com"},
		},
		{
			name: "layer with time",
			layer: &Layer{
				Type: LayerTypeAdd,
				Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantOut: []string{"@layer", "@time 2024-01-01T00:00:00Z"},
		},
		{
			name: "layer with note",
			layer: &Layer{
				Type: LayerTypeAdd,
				Note: "Test note",
			},
			wantOut: []string{"@layer", "@note Test note"},
		},
		{
			name: "layer with data",
			layer: &Layer{
				Type: LayerTypeAdd,
				Data: mustParseC4("c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN"),
			},
			wantOut: []string{"@layer", "@data c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN"},
		},
		{
			name: "layer with all fields",
			layer: &Layer{
				Type: LayerTypeRemove,
				By:   "admin@example.com",
				Time: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
				Note: "Cleanup old files",
				Data: mustParseC4("c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN"),
			},
			wantOut: []string{
				"@remove",
				"@by admin@example.com",
				"@time 2024-06-15T10:30:00Z",
				"@note Cleanup old files",
				"@data c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				Version: "1.0",
				Layers:  []*Layer{tt.layer},
			}
			var buf bytes.Buffer

			err := NewEncoder(&buf).Encode(m)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			output := buf.String()
			for _, want := range tt.wantOut {
				if !strings.Contains(output, want) {
					t.Errorf("Output missing %q\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestManifestWithLayers(t *testing.T) {
	m := &Manifest{
		Version: "1.0",
		Layers: []*Layer{
			{
				Type: LayerTypeAdd,
				By:   "user@example.com",
			},
		},
		Entries: []*Entry{
			{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      100,
				Name:      "new.txt",
			},
		},
	}

	var buf bytes.Buffer
	err := NewEncoder(&buf).Encode(m)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encode() wrote 0 bytes")
	}

	output := buf.String()
	expected := []string{
		"@c4m 1.0",
		"@layer",
		"@by user@example.com",
		"-rw-r--r-- 2024-01-01T00:00:00Z 100 new.txt",
	}
	
	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Errorf("Output missing %q\nGot:\n%s", want, output)
		}
	}
}

// Helper function to parse C4 ID
func mustParseC4(s string) c4.ID {
	id, err := c4.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

func TestManifestValidate(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	
	tests := []struct {
		name    string
		manifest *Manifest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid manifest",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Name:      "file1.txt",
						Timestamp: testTime,
						Size:      100,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing version",
			manifest: &Manifest{
				Entries: []*Entry{},
			},
			wantErr: true,
			errMsg:  "missing version",
		},
		{
			name: "duplicate paths",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "file1.txt", Timestamp: testTime, Size: 100},
					{Name: "file1.txt", Timestamp: testTime, Size: 200},
				},
			},
			wantErr: true,
			errMsg:  "duplicate path",
		},
		{
			name: "empty name",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "", Timestamp: testTime, Size: 100},
				},
			},
			wantErr: true,
			errMsg:  "empty name",
		},
		{
			name: "null timestamp is valid",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "file.txt", Size: 100},
				},
			},
			wantErr: false,
		},
		{
			name: "null size is valid",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "file.txt", Timestamp: testTime, Size: -1},
				},
			},
			wantErr: false,
		},
		{
			name: "path traversal ../",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "../file.txt", Timestamp: testTime, Size: 100},
				},
			},
			wantErr: true,
			errMsg:  "path traversal",
		},
		{
			name: "path traversal ./",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "./file.txt", Timestamp: testTime, Size: 100},
				},
			},
			wantErr: true,
			errMsg:  "path traversal",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error = %q, want containing %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestManifestHasNullValues(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		entries []*Entry
		want    bool
	}{
		{
			name:    "empty manifest",
			entries: nil,
			want:    false,
		},
		{
			name: "all valid entries",
			entries: []*Entry{
				{Mode: 0644, Timestamp: testTime, Size: 100, Name: "a.txt"},
				{Mode: 0644, Timestamp: testTime, Size: 200, Name: "b.txt"},
			},
			want: false,
		},
		{
			name: "one entry with null size",
			entries: []*Entry{
				{Mode: 0644, Timestamp: testTime, Size: 100, Name: "a.txt"},
				{Mode: 0644, Timestamp: testTime, Size: -1, Name: "b.txt"},
			},
			want: true,
		},
		{
			name: "one entry with null timestamp",
			entries: []*Entry{
				{Mode: 0644, Timestamp: time.Unix(0, 0), Size: 100, Name: "a.txt"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{Version: "1.0", Entries: tt.entries}
			if got := m.HasNullValues(); got != tt.want {
				t.Errorf("HasNullValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManifestGetDataBlock(t *testing.T) {
	// Create test IDs
	id1 := c4.Identify(strings.NewReader("content1"))
	id2 := c4.Identify(strings.NewReader("content2"))
	id3 := c4.Identify(strings.NewReader("content3"))

	block1 := &DataBlock{ID: id1, Content: []byte("content1")}
	block2 := &DataBlock{ID: id2, Content: []byte("content2")}

	m := &Manifest{
		Version:    "1.0",
		DataBlocks: []*DataBlock{block1, block2},
	}

	// Test finding existing blocks
	if got := m.GetDataBlock(id1); got != block1 {
		t.Errorf("GetDataBlock(id1) = %v, want %v", got, block1)
	}
	if got := m.GetDataBlock(id2); got != block2 {
		t.Errorf("GetDataBlock(id2) = %v, want %v", got, block2)
	}

	// Test not found
	if got := m.GetDataBlock(id3); got != nil {
		t.Errorf("GetDataBlock(id3) = %v, want nil", got)
	}
}

func TestManifestGetIDList(t *testing.T) {
	// Create an ID list
	idList := newIDList()
	id1 := c4.Identify(strings.NewReader("file1"))
	id2 := c4.Identify(strings.NewReader("file2"))
	idList.Add(id1)
	idList.Add(id2)

	// Create a data block from the ID list
	block := createDataBlockFromIDList(idList)

	m := &Manifest{
		Version:    "1.0",
		DataBlocks: []*DataBlock{block},
	}

	// Test getting the ID list
	got, err := m.getIDList(block.ID)
	if err != nil {
		t.Fatalf("getIDList() error = %v", err)
	}
	if got.Count() != 2 {
		t.Errorf("getIDList() count = %d, want 2", got.Count())
	}

	// Test not found
	unknownID := c4.Identify(strings.NewReader("unknown"))
	_, err = m.getIDList(unknownID)
	if err == nil {
		t.Error("getIDList(unknown) should return error")
	}
}

func TestCanonicalize(t *testing.T) {
	t.Run("sets default mode for files", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{
			Name:      "file.txt",
			Size:      100,
			Timestamp: time.Now().UTC(),
			Mode:      0, // Null mode
			C4ID:      c4.Identify(strings.NewReader("test")),
		})

		m.Canonicalize()

		if m.Entries[0].Mode != 0644 {
			t.Errorf("expected mode 0644, got %o", m.Entries[0].Mode)
		}
	})

	t.Run("sets default mode for directories", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{
			Name:      "dir/",
			Size:      0,
			Timestamp: time.Now().UTC(),
			Mode:      0, // Null mode - will be detected as dir from name ending in /
			C4ID:      c4.ID{},
		})

		m.Canonicalize()

		expectedMode := os.ModeDir | 0755
		if m.Entries[0].Mode != expectedMode {
			t.Errorf("expected mode %o, got %o", expectedMode, m.Entries[0].Mode)
		}
	})

	t.Run("null timestamps stay null after canonicalize", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{
			Name:      "file.txt",
			Size:      100,
			Timestamp: NullTimestamp(),
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader("test")),
		})

		m.Canonicalize()

		if !m.Entries[0].Timestamp.Equal(NullTimestamp()) {
			t.Errorf("expected null timestamp to stay null, got %v", m.Entries[0].Timestamp)
		}
	})

	t.Run("sets default size for negative size", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{
			Name:      "file.txt",
			Size:      -1, // Null size
			Timestamp: time.Now().UTC(),
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader("test")),
		})

		m.Canonicalize()

		if m.Entries[0].Size != 0 {
			t.Errorf("expected size 0, got %d", m.Entries[0].Size)
		}
	})

	t.Run("propagates metadata from children to parents", func(t *testing.T) {
		now := time.Now().UTC()
		m := NewManifest()

		// Add directory with null values
		m.AddEntry(&Entry{
			Name:      "dir/",
			Size:      -1,
			Timestamp: time.Unix(0, 0),
			Mode:      os.ModeDir,
			Depth:     0,
		})

		// Add child file with real values
		m.AddEntry(&Entry{
			Name:      "dir/file.txt",
			Size:      100,
			Timestamp: now,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader("test")),
			Depth:     1,
		})

		m.Canonicalize()

		// Directory should have size propagated
		if m.Entries[0].Size != 100 {
			t.Errorf("expected dir size 100, got %d", m.Entries[0].Size)
		}
	})
}
// ----------------------------------------------------------------------------
// Propagate Tests (merged from propagate_test.go)
// ----------------------------------------------------------------------------

func TestGetDirectoryChildren(t *testing.T) {
	// Create a hierarchy:
	// root/       (depth 0)
	//   file1.txt (depth 1)
	//   subdir/   (depth 1)
	//     file2.txt (depth 2)
	//   file3.txt (depth 1)
	// other/      (depth 0)

	root := &Entry{Name: "root/", Mode: os.ModeDir, Depth: 0}
	file1 := &Entry{Name: "file1.txt", Size: 100, Depth: 1}
	subdir := &Entry{Name: "subdir/", Mode: os.ModeDir, Depth: 1}
	file2 := &Entry{Name: "file2.txt", Size: 200, Depth: 2}
	file3 := &Entry{Name: "file3.txt", Size: 300, Depth: 1}
	other := &Entry{Name: "other/", Mode: os.ModeDir, Depth: 0}

	entries := []*Entry{root, file1, subdir, file2, file3, other}

	tests := []struct {
		name     string
		dir      *Entry
		expected []*Entry
	}{
		{
			name:     "root children",
			dir:      root,
			expected: []*Entry{file1, subdir, file3},
		},
		{
			name:     "subdir children",
			dir:      subdir,
			expected: []*Entry{file2},
		},
		{
			name:     "other children (empty)",
			dir:      other,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			children := getDirectoryChildren(entries, tt.dir)
			if len(children) != len(tt.expected) {
				t.Errorf("got %d children, want %d", len(children), len(tt.expected))
				return
			}
			for i, child := range children {
				if child != tt.expected[i] {
					t.Errorf("child %d: got %s, want %s", i, child.Name, tt.expected[i].Name)
				}
			}
		})
	}
}

func TestCalculateDirectorySize(t *testing.T) {
	tests := []struct {
		name     string
		entries  []*Entry
		expected int64
	}{
		{
			name:     "empty",
			entries:  []*Entry{},
			expected: 0,
		},
		{
			name: "single file",
			entries: []*Entry{
				{Name: "file.txt", Size: 100},
			},
			expected: 100,
		},
		{
			name: "multiple files",
			entries: []*Entry{
				{Name: "a.txt", Size: 100},
				{Name: "b.txt", Size: 200},
				{Name: "c.txt", Size: 300},
			},
			expected: 600,
		},
		{
			name: "with null sizes",
			entries: []*Entry{
				{Name: "a.txt", Size: 100},
				{Name: "b.txt", Size: -1}, // null
				{Name: "c.txt", Size: 300},
			},
			expected: 400,
		},
		{
			name: "all null sizes",
			entries: []*Entry{
				{Name: "a.txt", Size: -1},
				{Name: "b.txt", Size: -1},
			},
			expected: 0,
		},
		{
			name: "zero size files",
			entries: []*Entry{
				{Name: "empty.txt", Size: 0},
				{Name: "also_empty.txt", Size: 0},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateDirectorySize(tt.entries)
			if result != tt.expected {
				t.Errorf("got %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestGetMostRecentModtime(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		entries  []*Entry
		expected time.Time
	}{
		{
			name:     "empty returns null timestamp",
			entries:  []*Entry{},
			expected: NullTimestamp(),
		},
		{
			name: "single timestamp",
			entries: []*Entry{
				{Name: "a.txt", Timestamp: t1},
			},
			expected: t1,
		},
		{
			name: "multiple timestamps",
			entries: []*Entry{
				{Name: "a.txt", Timestamp: t1},
				{Name: "b.txt", Timestamp: t2},
				{Name: "c.txt", Timestamp: t3},
			},
			expected: t2, // most recent
		},
		{
			name: "with null timestamps",
			entries: []*Entry{
				{Name: "a.txt", Timestamp: t1},
				{Name: "b.txt", Timestamp: time.Unix(0, 0)}, // null (epoch)
				{Name: "c.txt", Timestamp: t3},
			},
			expected: t3,
		},
		{
			name: "all null timestamps returns null timestamp",
			entries: []*Entry{
				{Name: "a.txt", Timestamp: time.Unix(0, 0)},
				{Name: "b.txt", Timestamp: time.Unix(0, 0)},
			},
			expected: NullTimestamp(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMostRecentModtime(tt.entries)
			if !result.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPropagateMetadata(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)

	t.Run("propagates size to directory", func(t *testing.T) {
		dir := &Entry{Name: "dir/", Mode: os.ModeDir, Size: -1, Timestamp: t1, Depth: 0}
		file1 := &Entry{Name: "a.txt", Size: 100, Timestamp: t1, Depth: 1}
		file2 := &Entry{Name: "b.txt", Size: 200, Timestamp: t1, Depth: 1}

		entries := []*Entry{dir, file1, file2}
		propagateMetadata(entries)

		if dir.Size != 300 {
			t.Errorf("dir size: got %d, want 300", dir.Size)
		}
	})

	t.Run("propagates timestamp to directory", func(t *testing.T) {
		dir := &Entry{Name: "dir/", Mode: os.ModeDir, Size: 0, Timestamp: time.Unix(0, 0), Depth: 0}
		file1 := &Entry{Name: "a.txt", Size: 100, Timestamp: t1, Depth: 1}
		file2 := &Entry{Name: "b.txt", Size: 200, Timestamp: t2, Depth: 1}
		file3 := &Entry{Name: "c.txt", Size: 300, Timestamp: t3, Depth: 1}

		entries := []*Entry{dir, file1, file2, file3}
		propagateMetadata(entries)

		if !dir.Timestamp.Equal(t2) {
			t.Errorf("dir timestamp: got %v, want %v", dir.Timestamp, t2)
		}
	})

	t.Run("propagates both size and timestamp", func(t *testing.T) {
		dir := &Entry{Name: "dir/", Mode: os.ModeDir, Size: -1, Timestamp: time.Unix(0, 0), Depth: 0}
		file1 := &Entry{Name: "a.txt", Size: 100, Timestamp: t1, Depth: 1}
		file2 := &Entry{Name: "b.txt", Size: 200, Timestamp: t2, Depth: 1}

		entries := []*Entry{dir, file1, file2}
		propagateMetadata(entries)

		if dir.Size != 300 {
			t.Errorf("dir size: got %d, want 300", dir.Size)
		}
		if !dir.Timestamp.Equal(t2) {
			t.Errorf("dir timestamp: got %v, want %v", dir.Timestamp, t2)
		}
	})

	t.Run("does not overwrite explicit values", func(t *testing.T) {
		explicitTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		dir := &Entry{Name: "dir/", Mode: os.ModeDir, Size: 999, Timestamp: explicitTime, Depth: 0}
		file1 := &Entry{Name: "a.txt", Size: 100, Timestamp: t2, Depth: 1}

		entries := []*Entry{dir, file1}
		propagateMetadata(entries)

		// Values should remain unchanged
		if dir.Size != 999 {
			t.Errorf("dir size should not change: got %d, want 999", dir.Size)
		}
		if !dir.Timestamp.Equal(explicitTime) {
			t.Errorf("dir timestamp should not change: got %v, want %v", dir.Timestamp, explicitTime)
		}
	})

	t.Run("handles nested directories", func(t *testing.T) {
		root := &Entry{Name: "root/", Mode: os.ModeDir, Size: -1, Timestamp: time.Unix(0, 0), Depth: 0}
		subdir := &Entry{Name: "sub/", Mode: os.ModeDir, Size: -1, Timestamp: time.Unix(0, 0), Depth: 1}
		file1 := &Entry{Name: "a.txt", Size: 100, Timestamp: t1, Depth: 2}
		file2 := &Entry{Name: "b.txt", Size: 200, Timestamp: t2, Depth: 1}

		entries := []*Entry{root, subdir, file1, file2}
		propagateMetadata(entries)

		// subdir should get file1's info (its only direct child)
		if subdir.Size != 100 {
			t.Errorf("subdir size: got %d, want 100", subdir.Size)
		}
		if !subdir.Timestamp.Equal(t1) {
			t.Errorf("subdir timestamp: got %v, want %v", subdir.Timestamp, t1)
		}

		// With reverse-order iteration, subdir is resolved first,
		// so root correctly includes subdir's propagated size + file2's size
		if root.Size != 300 {
			t.Errorf("root size: got %d, want 300 (subdir=100 + file2=200)", root.Size)
		}
		// root timestamp should be t2 (most recent of direct children)
		if !root.Timestamp.Equal(t2) {
			t.Errorf("root timestamp: got %v, want %v", root.Timestamp, t2)
		}
	})

	t.Run("skips non-directory entries", func(t *testing.T) {
		file := &Entry{Name: "file.txt", Size: -1, Timestamp: time.Unix(0, 0), Depth: 0}
		entries := []*Entry{file}
		propagateMetadata(entries)

		// File should not be modified (null values preserved)
		if file.Size != -1 {
			t.Errorf("file size should remain null: got %d", file.Size)
		}
	})

	t.Run("handles empty entries", func(t *testing.T) {
		entries := []*Entry{}
		propagateMetadata(entries) // Should not panic
	})
}

func TestManifest_Copy_DeepCopyDataBlocks(t *testing.T) {
	// Create a manifest with DataBlocks
	original := NewManifest()
	original.AddEntry(&Entry{Name: "file.txt", Size: 100})

	content := []byte("original content")
	id := c4.Identify(bytes.NewReader(content))
	original.AddDataBlock(&DataBlock{
		ID:       id,
		Content:  content,
		IsIDList: false,
	})

	// Copy the manifest
	cp := original.Copy()

	// Verify the copy has the data block
	if len(cp.DataBlocks) != 1 {
		t.Fatalf("expected 1 data block in copy, got %d", len(cp.DataBlocks))
	}

	// Mutate the original's DataBlock content
	original.DataBlocks[0].Content[0] = 'X'
	original.DataBlocks[0].IsIDList = true

	// The copy must be unaffected
	if cp.DataBlocks[0].Content[0] == 'X' {
		t.Error("Copy() DataBlock Content shares backing array with original")
	}
	if string(cp.DataBlocks[0].Content) != "original content" {
		t.Errorf("Copy() DataBlock Content = %q, want %q", cp.DataBlocks[0].Content, "original content")
	}
	if cp.DataBlocks[0].IsIDList != false {
		t.Error("Copy() DataBlock fields should not be affected by original mutation")
	}

	// Verify they are different pointers
	if cp.DataBlocks[0] == original.DataBlocks[0] {
		t.Error("Copy() DataBlock pointers should differ")
	}
}
