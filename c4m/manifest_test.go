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
	
	m.Sort()
	
	// Check natural sort order - directories come first
	expected := []string{"dir1/", "file1.txt", "file2.txt", "file10.txt"}
	for i, e := range m.Entries {
		if e.Name != expected[i] {
			t.Errorf("Entry[%d] = %q, want %q", i, e.Name, expected[i])
		}
	}
}

func TestManifestWriteTo(t *testing.T) {
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
			n, err := tt.manifest.WriteTo(&buf)
			if err != nil {
				t.Fatalf("WriteTo() error = %v", err)
			}
			if n != int64(len(tt.want)) {
				t.Errorf("WriteTo() wrote %d bytes, want %d", n, len(tt.want))
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("WriteTo() = %q, want %q", got, tt.want)
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

func TestWriteLayer(t *testing.T) {
	tests := []struct {
		name     string
		layer    *Layer
		wantOut  []string
		wantErr  bool
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
			m := &Manifest{}
			var buf bytes.Buffer
			
			n, err := m.writeLayer(&buf, tt.layer)
			if (err != nil) != tt.wantErr {
				t.Errorf("writeLayer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if n == 0 && len(tt.wantOut) > 0 {
				t.Error("writeLayer() wrote 0 bytes")
			}
			
			output := buf.String()
			for _, want := range tt.wantOut {
				if !strings.Contains(output, want) {
					t.Errorf("Output missing %q", want)
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
	n, err := m.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}
	if n == 0 {
		t.Error("WriteTo() wrote 0 bytes")
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
			name: "zero timestamp",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "file.txt", Size: 100},
				},
			},
			wantErr: true,
			errMsg:  "zero timestamp",
		},
		{
			name: "negative size",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{Name: "file.txt", Timestamp: testTime, Size: -1},
				},
			},
			wantErr: true,
			errMsg:  "negative size",
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
	idList := NewIDList()
	id1 := c4.Identify(strings.NewReader("file1"))
	id2 := c4.Identify(strings.NewReader("file2"))
	idList.Add(id1)
	idList.Add(id2)

	// Create a data block from the ID list
	block := CreateDataBlockFromIDList(idList)

	m := &Manifest{
		Version:    "1.0",
		DataBlocks: []*DataBlock{block},
	}

	// Test getting the ID list
	got, err := m.GetIDList(block.ID)
	if err != nil {
		t.Fatalf("GetIDList() error = %v", err)
	}
	if got.Count() != 2 {
		t.Errorf("GetIDList() count = %d, want 2", got.Count())
	}

	// Test not found
	unknownID := c4.Identify(strings.NewReader("unknown"))
	_, err = m.GetIDList(unknownID)
	if err == nil {
		t.Error("GetIDList(unknown) should return error")
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

	t.Run("sets default timestamp for null timestamp", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{
			Name:      "file.txt",
			Size:      100,
			Timestamp: time.Unix(0, 0), // Null timestamp
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader("test")),
		})

		before := time.Now().UTC()
		m.Canonicalize()
		after := time.Now().UTC()

		ts := m.Entries[0].Timestamp
		if ts.Before(before) || ts.After(after) {
			t.Errorf("expected timestamp between %v and %v, got %v", before, after, ts)
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