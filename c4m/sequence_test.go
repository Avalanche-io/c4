package c4m

import (
	"fmt"
	"reflect"
	"testing"
	"time"
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

