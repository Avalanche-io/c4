package c4m

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestSequenceDetector(t *testing.T) {
	// Create manifest with sequences
	manifest := NewManifest()

	// Add a sequence of numbered files
	for i := 1; i <= 10; i++ {
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: time.Now(),
			Mode:      0644,
		})
	}

	// Add non-sequence files
	manifest.AddEntry(&Entry{
		Name:      "readme.txt",
		Size:      100,
		Timestamp: time.Now(),
		Mode:      0644,
	})

	// Detect sequences
	detector := NewSequenceDetector(3)
	result := detector.DetectSequences(manifest)

	// Should have collapsed the sequence
	if len(result.Entries) != 2 {
		t.Errorf("Expected 2 entries (1 sequence + 1 file), got %d", len(result.Entries))
	}

	// Check sequence notation
	foundSequence := false
	for _, entry := range result.Entries {
		if entry.IsSequence {
			if entry.Pattern != "frame.[0001-0010].exr" {
				t.Errorf("Expected pattern 'frame.[0001-0010].exr', got '%s'", entry.Pattern)
			}
			foundSequence = true
		}
	}

	if !foundSequence {
		t.Error("Sequence not detected")
	}
}

func TestSequenceExpansion(t *testing.T) {
	// Create manifest with sequence notation
	manifest := NewManifest()
	manifest.AddEntry(&Entry{
		Name:       "shot.[001-005].dpx",
		Size:       2048,
		Timestamp:  time.Now(),
		Mode:       0644,
		IsSequence: true,
		Pattern:    "shot.[001-005].dpx",
	})

	// Expand sequences
	expander := NewSequenceExpander(SequenceEmbedded)
	expanded, _, err := expander.ExpandManifest(manifest)
	if err != nil {
		t.Fatalf("Failed to expand manifest: %v", err)
	}

	// Should have 6 entries: 1 sequence notation + 5 expanded files
	if len(expanded.Entries) != 6 {
		t.Errorf("Expected 6 entries, got %d", len(expanded.Entries))
	}

	// Verify expanded files
	expectedFiles := []string{
		"shot.001.dpx",
		"shot.002.dpx",
		"shot.003.dpx",
		"shot.004.dpx",
		"shot.005.dpx",
	}

	foundFiles := make(map[string]bool)
	for _, entry := range expanded.Entries {
		foundFiles[entry.Name] = true
	}

	for _, expected := range expectedFiles {
		if !foundFiles[expected] {
			t.Errorf("Expected file %s not found in expansion", expected)
		}
	}
}

func TestParseSequence(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    *Sequence
		wantErr bool
	}{
		{
			name:    "simple_range",
			pattern: "frame.[0001-0100].exr",
			want: &Sequence{
				Prefix:  "frame.",
				Suffix:  ".exr",
				Padding: 4,
				Ranges: []Range{
					{Start: 1, End: 100, Step: 1},
				},
			},
			wantErr: false,
		},
		{
			name:    "stepped_range",
			pattern: "render.[0001-0100:2].png",
			want: &Sequence{
				Prefix:  "render.",
				Suffix:  ".png",
				Padding: 4,
				Ranges: []Range{
					{Start: 1, End: 100, Step: 2},
				},
			},
			wantErr: false,
		},
		{
			name:    "multiple_ranges",
			pattern: "shot.[01-50,75-100].dpx",
			want: &Sequence{
				Prefix:  "shot.",
				Suffix:  ".dpx",
				Padding: 2,
				Ranges: []Range{
					{Start: 1, End: 50, Step: 1},
					{Start: 75, End: 100, Step: 1},
				},
			},
			wantErr: false,
		},
		{
			name:    "individual_frames",
			pattern: "frame.[001,005,010,015].exr",
			want: &Sequence{
				Prefix:  "frame.",
				Suffix:  ".exr",
				Padding: 3,
				Ranges: []Range{
					{Start: 1, End: 1, Step: 1},
					{Start: 5, End: 5, Step: 1},
					{Start: 10, End: 10, Step: 1},
					{Start: 15, End: 15, Step: 1},
				},
			},
			wantErr: false,
		},
		{
			name:    "space_in_filename",
			pattern: `my\ animation.[001-100].png`,
			want: &Sequence{
				Prefix:  "my animation.",
				Suffix:  ".png",
				Padding: 3,
				Ranges: []Range{
					{Start: 1, End: 100, Step: 1},
				},
			},
			wantErr: false,
		},
		{
			name:    "no_sequence",
			pattern: "regular_file.txt",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid_range",
			pattern: "frame.[100-1].exr",
			want:    nil,
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSequence(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSequence() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSequence() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSequenceExpand(t *testing.T) {
	tests := []struct {
		name     string
		sequence *Sequence
		want     []string
	}{
		{
			name: "simple_range",
			sequence: &Sequence{
				Prefix:  "frame.",
				Suffix:  ".exr",
				Padding: 4,
				Ranges: []Range{
					{Start: 1, End: 3, Step: 1},
				},
			},
			want: []string{
				"frame.0001.exr",
				"frame.0002.exr",
				"frame.0003.exr",
			},
		},
		{
			name: "stepped_range",
			sequence: &Sequence{
				Prefix:  "render.",
				Suffix:  ".png",
				Padding: 3,
				Ranges: []Range{
					{Start: 1, End: 10, Step: 3},
				},
			},
			want: []string{
				"render.001.png",
				"render.004.png",
				"render.007.png",
				"render.010.png",
			},
		},
		{
			name: "multiple_ranges",
			sequence: &Sequence{
				Prefix:  "shot.",
				Suffix:  ".dpx",
				Padding: 2,
				Ranges: []Range{
					{Start: 1, End: 2, Step: 1},
					{Start: 5, End: 6, Step: 1},
				},
			},
			want: []string{
				"shot.01.dpx",
				"shot.02.dpx",
				"shot.05.dpx",
				"shot.06.dpx",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sequence.Expand()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Expand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSequenceCount(t *testing.T) {
	tests := []struct {
		name     string
		sequence *Sequence
		want     int
	}{
		{
			name: "simple_range",
			sequence: &Sequence{
				Ranges: []Range{
					{Start: 1, End: 100, Step: 1},
				},
			},
			want: 100,
		},
		{
			name: "stepped_range",
			sequence: &Sequence{
				Ranges: []Range{
					{Start: 1, End: 100, Step: 2},
				},
			},
			want: 50,
		},
		{
			name: "multiple_ranges",
			sequence: &Sequence{
				Ranges: []Range{
					{Start: 1, End: 50, Step: 1},
					{Start: 75, End: 100, Step: 1},
				},
			},
			want: 76, // 50 + 26
		},
		{
			name: "individual_frames",
			sequence: &Sequence{
				Ranges: []Range{
					{Start: 1, End: 1, Step: 1},
					{Start: 5, End: 5, Step: 1},
					{Start: 10, End: 10, Step: 1},
				},
			},
			want: 3,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sequence.Count()
			if got != tt.want {
				t.Errorf("Count() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSequenceContains(t *testing.T) {
	sequence := &Sequence{
		Ranges: []Range{
			{Start: 1, End: 10, Step: 1},
			{Start: 20, End: 30, Step: 2},
		},
	}
	
	tests := []struct {
		frame int
		want  bool
	}{
		{1, true},   // In first range
		{5, true},   // In first range
		{10, true},  // End of first range
		{11, false}, // Between ranges
		{20, true},  // Start of second range
		{21, false}, // Not on step in second range
		{22, true},  // On step in second range
		{30, true},  // End of second range
		{31, false}, // After all ranges
		{0, false},  // Before all ranges
	}
	
	for _, tt := range tests {
		t.Run(string(rune(tt.frame)), func(t *testing.T) {
			got := sequence.Contains(tt.frame)
			if got != tt.want {
				t.Errorf("Contains(%d) = %v, want %v", tt.frame, got, tt.want)
			}
		})
	}
}

func TestIsSequence(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"frame.[0001-0100].exr", true},
		{"render.[01-50,75-100].png", true},
		{"shot.[001].dpx", true},
		{"regular_file.txt", false},
		{"file_with_brackets[but_no_numbers].txt", false},
		{"file[].txt", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := IsSequence(tt.pattern)
			if got != tt.want {
				t.Errorf("IsSequence(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestExpandSequencePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    []string
		wantErr bool
	}{
		{
			name:    "simple expansion",
			pattern: "frame.[01-03].exr",
			want: []string{
				"frame.01.exr",
				"frame.02.exr",
				"frame.03.exr",
			},
		},
		{
			name:    "no pattern",
			pattern: "regular_file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandSequencePattern(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandSequencePattern() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandSequencePattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSequenceDetectorMinLength(t *testing.T) {
	t.Run("enforces minimum length of 2", func(t *testing.T) {
		// Test with minLength < 2
		detector := NewSequenceDetector(1)
		if detector.minSequenceLength != 2 {
			t.Errorf("expected minSequenceLength 2, got %d", detector.minSequenceLength)
		}

		detector = NewSequenceDetector(0)
		if detector.minSequenceLength != 2 {
			t.Errorf("expected minSequenceLength 2, got %d", detector.minSequenceLength)
		}

		detector = NewSequenceDetector(-5)
		if detector.minSequenceLength != 2 {
			t.Errorf("expected minSequenceLength 2, got %d", detector.minSequenceLength)
		}
	})

	t.Run("accepts minLength >= 2", func(t *testing.T) {
		detector := NewSequenceDetector(5)
		if detector.minSequenceLength != 5 {
			t.Errorf("expected minSequenceLength 5, got %d", detector.minSequenceLength)
		}
	})
}

func TestFindRanges(t *testing.T) {
	detector := NewSequenceDetector(2)

	t.Run("empty frames", func(t *testing.T) {
		ranges := detector.findRanges([]int{})
		if ranges != nil {
			t.Errorf("expected nil for empty frames, got %v", ranges)
		}
	})

	t.Run("single frame", func(t *testing.T) {
		ranges := detector.findRanges([]int{5})
		if len(ranges) != 1 {
			t.Fatalf("expected 1 range, got %d", len(ranges))
		}
		if ranges[0].start != 5 || ranges[0].end != 5 || ranges[0].count != 1 {
			t.Errorf("expected range {5, 5, 1}, got %+v", ranges[0])
		}
	})

	t.Run("continuous range", func(t *testing.T) {
		ranges := detector.findRanges([]int{1, 2, 3, 4, 5})
		if len(ranges) != 1 {
			t.Fatalf("expected 1 range, got %d", len(ranges))
		}
		if ranges[0].start != 1 || ranges[0].end != 5 || ranges[0].count != 5 {
			t.Errorf("expected range {1, 5, 5}, got %+v", ranges[0])
		}
	})

	t.Run("gap in sequence", func(t *testing.T) {
		ranges := detector.findRanges([]int{1, 2, 3, 10, 11, 12})
		if len(ranges) != 2 {
			t.Fatalf("expected 2 ranges, got %d", len(ranges))
		}
		if ranges[0].start != 1 || ranges[0].end != 3 || ranges[0].count != 3 {
			t.Errorf("expected first range {1, 3, 3}, got %+v", ranges[0])
		}
		if ranges[1].start != 10 || ranges[1].end != 12 || ranges[1].count != 3 {
			t.Errorf("expected second range {10, 12, 3}, got %+v", ranges[1])
		}
	})

	t.Run("multiple gaps", func(t *testing.T) {
		ranges := detector.findRanges([]int{1, 5, 6, 10})
		if len(ranges) != 3 {
			t.Fatalf("expected 3 ranges, got %d", len(ranges))
		}
		// Range 1: just frame 1
		if ranges[0].start != 1 || ranges[0].end != 1 {
			t.Errorf("expected first range {1, 1}, got %+v", ranges[0])
		}
		// Range 2: frames 5-6
		if ranges[1].start != 5 || ranges[1].end != 6 {
			t.Errorf("expected second range {5, 6}, got %+v", ranges[1])
		}
		// Range 3: just frame 10
		if ranges[2].start != 10 || ranges[2].end != 10 {
			t.Errorf("expected third range {10, 10}, got %+v", ranges[2])
		}
	})
}

func TestSequenceExpanderStandalone(t *testing.T) {
	// Test standalone mode where expansions go to separate manifest
	manifest := NewManifest()
	manifest.AddEntry(&Entry{
		Name:       "shot.[001-003].dpx",
		Size:       2048,
		Timestamp:  time.Now(),
		Mode:       0644,
		IsSequence: true,
		Pattern:    "shot.[001-003].dpx",
	})

	expander := NewSequenceExpander(SequenceStandalone)
	main, expansions, err := expander.ExpandManifest(manifest)
	if err != nil {
		t.Fatalf("ExpandManifest() error = %v", err)
	}

	// Main manifest should have 1 entry (the sequence notation)
	if len(main.Entries) != 1 {
		t.Errorf("expected 1 entry in main manifest, got %d", len(main.Entries))
	}

	// Expansions manifest should have 3 entries
	if expansions == nil {
		t.Fatal("expected expansions manifest, got nil")
	}
	if len(expansions.Entries) != 3 {
		t.Errorf("expected 3 entries in expansions manifest, got %d", len(expansions.Entries))
	}
}

func TestExpandSequenceEntryWithManifestNilC4ID(t *testing.T) {
	// Test with nil C4ID (uses legacy behavior - entry's C4ID for all)
	manifest := NewManifest()
	entry := &Entry{
		Name:       "frame.[01-03].exr",
		Size:       1024,
		Timestamp:  time.Now(),
		Mode:       0644,
		IsSequence: true,
		Pattern:    "frame.[01-03].exr",
		// C4ID is nil
	}

	expanded, err := ExpandSequenceEntryWithManifest(entry, manifest)
	if err != nil {
		t.Fatalf("ExpandSequenceEntryWithManifest() error = %v", err)
	}

	if len(expanded) != 3 {
		t.Errorf("expected 3 expanded entries, got %d", len(expanded))
	}

	// All entries should have nil C4ID (since entry.C4ID was nil)
	for _, e := range expanded {
		if !e.C4ID.IsNil() {
			t.Errorf("expected nil C4ID, got %s", e.C4ID)
		}
	}
}


// ----------------------------------------------------------------------------
// IDList Tests (merged from idlist_test.go)
// ----------------------------------------------------------------------------

func TestIDList(t *testing.T) {
	t.Run("NewIDList creates empty list", func(t *testing.T) {
		list := NewIDList()
		if list.Count() != 0 {
			t.Errorf("expected 0 items, got %d", list.Count())
		}
	})

	t.Run("Add and Get", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))

		list.Add(id1)
		list.Add(id2)

		if list.Count() != 2 {
			t.Errorf("expected 2 items, got %d", list.Count())
		}

		if list.Get(0) != id1 {
			t.Errorf("expected id1 at index 0")
		}
		if list.Get(1) != id2 {
			t.Errorf("expected id2 at index 1")
		}

		// Out of bounds returns nil ID
		if !list.Get(-1).IsNil() {
			t.Errorf("expected nil ID for negative index")
		}
		if !list.Get(100).IsNil() {
			t.Errorf("expected nil ID for out of bounds index")
		}
	})

	t.Run("Canonical format", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))

		list.Add(id1)
		list.Add(id2)

		canonical := list.Canonical()

		// Should have trailing newline on each line
		lines := strings.Split(strings.TrimSuffix(canonical, "\n"), "\n")
		if len(lines) != 2 {
			t.Errorf("expected 2 lines, got %d", len(lines))
		}

		if lines[0] != id1.String() {
			t.Errorf("expected %s, got %s", id1.String(), lines[0])
		}
		if lines[1] != id2.String() {
			t.Errorf("expected %s, got %s", id2.String(), lines[1])
		}
	})

	t.Run("ComputeC4ID", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		c4id := list.ComputeC4ID()
		if c4id.IsNil() {
			t.Errorf("expected non-nil C4 ID")
		}

		// Same list should produce same C4 ID
		list2 := NewIDList()
		list2.Add(id1)
		c4id2 := list2.ComputeC4ID()
		if c4id != c4id2 {
			t.Errorf("expected same C4 ID for same list")
		}
	})
}

func TestParseIDList(t *testing.T) {
	t.Run("parse valid ID list", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))

		input := id1.String() + "\n" + id2.String() + "\n"
		list, err := ParseIDListFromString(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if list.Count() != 2 {
			t.Errorf("expected 2 items, got %d", list.Count())
		}
	})

	t.Run("tolerant of whitespace", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))

		// Extra whitespace, blank lines
		input := "\n  " + id1.String() + "  \n\n"
		list, err := ParseIDListFromString(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if list.Count() != 1 {
			t.Errorf("expected 1 item, got %d", list.Count())
		}
	})

	t.Run("invalid ID format", func(t *testing.T) {
		input := "not-a-valid-c4-id\n"
		_, err := ParseIDListFromString(input)
		if err == nil {
			t.Errorf("expected error for invalid C4 ID")
		}
	})
}

func TestIsIDListContent(t *testing.T) {
	t.Run("valid ID list content", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		content := []byte(id1.String() + "\n")
		if !IsIDListContent(content) {
			t.Errorf("expected true for valid ID list content")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		if IsIDListContent([]byte("")) {
			t.Errorf("expected false for empty content")
		}
	})

	t.Run("non-ID content", func(t *testing.T) {
		content := []byte("hello world\n")
		if IsIDListContent(content) {
			t.Errorf("expected false for non-ID content")
		}
	})

	t.Run("mixed content", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		content := []byte(id1.String() + "\nhello\n")
		if IsIDListContent(content) {
			t.Errorf("expected false for mixed content")
		}
	})
}

func TestDataBlock(t *testing.T) {
	t.Run("parse ID list data block", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		listID := list.ComputeC4ID()

		block, err := ParseDataBlock(listID, content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !block.IsIDList {
			t.Errorf("expected IsIDList to be true")
		}

		if block.ID != listID {
			t.Errorf("expected ID %s, got %s", listID, block.ID)
		}
	})

	t.Run("parse base64 data block", func(t *testing.T) {
		// Some arbitrary non-ID content
		content := []byte("hello world")
		contentID := c4.Identify(bytes.NewReader(content))

		// Base64 encode
		encoded := "aGVsbG8gd29ybGQ=" // base64("hello world")

		block, err := ParseDataBlock(contentID, encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.IsIDList {
			t.Errorf("expected IsIDList to be false")
		}

		if string(block.Content) != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", string(block.Content))
		}
	})

	t.Run("content hash mismatch", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		wrongID := c4.Identify(strings.NewReader("wrong"))

		_, err := ParseDataBlock(wrongID, content)
		if err == nil {
			t.Errorf("expected error for hash mismatch")
		}
	})

	t.Run("GetIDList from block", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		listID := list.ComputeC4ID()

		block, err := ParseDataBlock(listID, content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		retrieved, err := block.GetIDList()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if retrieved.Count() != 1 {
			t.Errorf("expected 1 item, got %d", retrieved.Count())
		}
	})
}

func TestFormatDataBlock(t *testing.T) {
	t.Run("format ID list block", func(t *testing.T) {
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		block := CreateDataBlockFromIDList(list)
		formatted := FormatDataBlock(block)

		if !strings.HasPrefix(formatted, "@data ") {
			t.Errorf("expected to start with '@data '")
		}

		if !strings.Contains(formatted, id1.String()) {
			t.Errorf("expected to contain ID")
		}
	})
}

func TestCreateDataBlockFromIDList(t *testing.T) {
	list := NewIDList()
	id1 := c4.Identify(strings.NewReader("test1"))
	id2 := c4.Identify(strings.NewReader("test2"))
	list.Add(id1)
	list.Add(id2)

	block := CreateDataBlockFromIDList(list)

	if !block.IsIDList {
		t.Errorf("expected IsIDList to be true")
	}

	// Verify content matches canonical form
	if string(block.Content) != list.Canonical() {
		t.Errorf("content mismatch")
	}

	// Verify ID is correct
	expectedID := list.ComputeC4ID()
	if block.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, block.ID)
	}
}

func TestFormatDataBlockIDList(t *testing.T) {
	list := NewIDList()
	id1 := c4.Identify(strings.NewReader("test1"))
	id2 := c4.Identify(strings.NewReader("test2"))
	list.Add(id1)
	list.Add(id2)

	block := CreateDataBlockFromIDList(list)
	formatted := FormatDataBlock(block)

	// Should start with @data directive
	if !strings.HasPrefix(formatted, "@data ") {
		t.Errorf("expected to start with @data, got %q", formatted[:20])
	}

	// Should contain the block ID
	if !strings.Contains(formatted, block.ID.String()) {
		t.Errorf("expected to contain block ID")
	}

	// Should contain the C4 IDs in plain text (not base64)
	if !strings.Contains(formatted, id1.String()) {
		t.Errorf("expected to contain id1 in plain text")
	}
	if !strings.Contains(formatted, id2.String()) {
		t.Errorf("expected to contain id2 in plain text")
	}
}

func TestFormatDataBlockBinary(t *testing.T) {
	// Create a non-ID list data block with binary content
	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}
	id := c4.Identify(bytes.NewReader(binaryContent))

	block := &DataBlock{
		ID:       id,
		Content:  binaryContent,
		IsIDList: false,
	}

	formatted := FormatDataBlock(block)

	// Should start with @data directive
	if !strings.HasPrefix(formatted, "@data ") {
		t.Errorf("expected to start with @data")
	}

	// Should contain base64 encoded content (AAECA//+)
	// {0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE} encodes to "AAECA//+"
	if !strings.Contains(formatted, "AAECA//+") {
		t.Errorf("expected base64 encoded content 'AAECA//+', got %q", formatted)
	}

	// Should NOT contain raw binary
	if strings.Contains(formatted, string(binaryContent)) {
		t.Errorf("should not contain raw binary content")
	}
}

func TestFormatDataBlockLongContent(t *testing.T) {
	// Create a data block with content longer than 76 chars to test line wrapping
	longContent := bytes.Repeat([]byte("ABCDEFGHIJ"), 20) // 200 bytes
	id := c4.Identify(bytes.NewReader(longContent))

	block := &DataBlock{
		ID:       id,
		Content:  longContent,
		IsIDList: false,
	}

	formatted := FormatDataBlock(block)

	// Split into lines and check line lengths
	lines := strings.Split(formatted, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // Skip @data directive line
		}
		if len(line) > 76 && line != "" {
			t.Errorf("line %d exceeds 76 chars: %d", i, len(line))
		}
	}
}

func TestDataBlockGetIDList(t *testing.T) {
	t.Run("returns error for non-ID list", func(t *testing.T) {
		block := &DataBlock{
			ID:       c4.Identify(strings.NewReader("test")),
			Content:  []byte("not an ID list"),
			IsIDList: false,
		}

		_, err := block.GetIDList()
		if err == nil {
			t.Error("expected error for non-ID list block")
		}
		if !strings.Contains(err.Error(), "not an ID list") {
			t.Errorf("expected 'not an ID list' error, got: %v", err)
		}
	})

	t.Run("returns ID list for valid block", func(t *testing.T) {
		// Create a valid ID list block
		list := NewIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))
		list.Add(id1)
		list.Add(id2)

		block := CreateDataBlockFromIDList(list)

		retrieved, err := block.GetIDList()
		if err != nil {
			t.Fatalf("GetIDList() error = %v", err)
		}

		if retrieved.Count() != 2 {
			t.Errorf("expected 2 IDs, got %d", retrieved.Count())
		}
		if retrieved.Get(0) != id1 {
			t.Errorf("expected first ID to be %s", id1)
		}
		if retrieved.Get(1) != id2 {
			t.Errorf("expected second ID to be %s", id2)
		}
	})
}
