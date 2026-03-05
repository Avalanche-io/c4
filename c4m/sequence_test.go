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
					{Start: 5, End: 15, Step: 5},
				},
			},
			wantErr: false,
		},
		{
			name:    "adjacent_ranges_merged",
			pattern: "comp.[0001-0100,0101-0200].exr",
			want: &Sequence{
				Prefix:  "comp.",
				Suffix:  ".exr",
				Padding: 4,
				Ranges: []Range{
					{Start: 1, End: 200, Step: 1},
				},
			},
			wantErr: false,
		},
		{
			name:    "three_adjacent_ranges_merged",
			pattern: "frame.[001-100,101-200,201-300].exr",
			want: &Sequence{
				Prefix:  "frame.",
				Suffix:  ".exr",
				Padding: 3,
				Ranges: []Range{
					{Start: 1, End: 300, Step: 1},
				},
			},
			wantErr: false,
		},
		{
			name:    "adjacent_stepped_ranges_merged",
			pattern: "render.[0001-0099:2,0101-0199:2].png",
			want: &Sequence{
				Prefix:  "render.",
				Suffix:  ".png",
				Padding: 4,
				Ranges: []Range{
					{Start: 1, End: 199, Step: 2},
				},
			},
			wantErr: false,
		},
		{
			name:    "different_steps_normalized",
			pattern: "render.[0001-0100:1,0101-0200:2].png",
			// Frames: 1..100 + 101,103,...,199. Contiguous: [1-101]. Singles: 103,105,...,199.
			want: &Sequence{
				Prefix:  "render.",
				Suffix:  ".png",
				Padding: 4,
				Ranges: []Range{
					{Start: 1, End: 101, Step: 1},
					{Start: 103, End: 199, Step: 2},
				},
			},
			wantErr: false,
		},
		{
			name:    "consecutive_singles_merged",
			pattern: "frame.[005,006,007].exr",
			want: &Sequence{
				Prefix:  "frame.",
				Suffix:  ".exr",
				Padding: 3,
				Ranges: []Range{
					{Start: 5, End: 7, Step: 1},
				},
			},
			wantErr: false,
		},
		{
			name:    "unsorted_ranges_normalized",
			pattern: "frame.[0101-0200,0001-0100].exr",
			want: &Sequence{
				Prefix:  "frame.",
				Suffix:  ".exr",
				Padding: 4,
				Ranges: []Range{
					{Start: 1, End: 200, Step: 1},
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

func TestNormalizeRanges(t *testing.T) {
	tests := []struct {
		name  string
		input []Range
		want  []Range
	}{
		{
			name:  "single_range_unchanged",
			input: []Range{{1, 100, 1}},
			want:  []Range{{1, 100, 1}},
		},
		{
			name:  "adjacent_step1_merged",
			input: []Range{{1, 100, 1}, {101, 200, 1}},
			want:  []Range{{1, 200, 1}},
		},
		{
			name:  "overlapping_ranges_merged",
			input: []Range{{1, 100, 1}, {50, 150, 1}},
			want:  []Range{{1, 150, 1}},
		},
		{
			name:  "subsumed_range_absorbed",
			input: []Range{{1, 100, 1}, {25, 75, 1}},
			want:  []Range{{1, 100, 1}},
		},
		{
			name:  "gap_preserved",
			input: []Range{{1, 50, 1}, {75, 100, 1}},
			want:  []Range{{1, 50, 1}, {75, 100, 1}},
		},
		{
			name:  "unsorted_input_sorted",
			input: []Range{{100, 200, 1}, {1, 99, 1}},
			want:  []Range{{1, 200, 1}},
		},
		{
			name:  "singles_to_contiguous",
			input: []Range{{1, 1, 1}, {2, 2, 1}, {3, 3, 1}, {4, 4, 1}, {5, 5, 1}},
			want:  []Range{{1, 5, 1}},
		},
		{
			name:  "singles_to_stepped",
			input: []Range{{2, 2, 1}, {4, 4, 1}, {6, 6, 1}, {8, 8, 1}, {10, 10, 1}},
			want:  []Range{{2, 10, 2}},
		},
		{
			name:  "mixed_contiguous_and_stepped",
			input: []Range{{1, 3, 1}, {5, 5, 1}, {7, 7, 1}, {9, 9, 1}},
			want:  []Range{{1, 3, 1}, {5, 9, 2}},
		},
		{
			name:  "stepped_fill_becomes_contiguous",
			input: []Range{{1, 9, 2}, {2, 2, 1}, {4, 4, 1}, {6, 6, 1}, {8, 8, 1}},
			want:  []Range{{1, 9, 1}},
		},
		{
			name:  "three_adjacent_step1",
			input: []Range{{1, 100, 1}, {101, 200, 1}, {201, 300, 1}},
			want:  []Range{{1, 300, 1}},
		},
		{
			name:  "random_frames_no_pattern",
			input: []Range{{3, 3, 1}, {7, 7, 1}, {15, 15, 1}, {22, 22, 1}},
			want:  []Range{{3, 3, 1}, {7, 7, 1}, {15, 15, 1}, {22, 22, 1}},
		},
		{
			name:  "two_singles_not_stepped",
			input: []Range{{10, 10, 1}, {20, 20, 1}},
			want:  []Range{{10, 10, 1}, {20, 20, 1}},
		},
		{
			name:  "three_singles_become_stepped",
			input: []Range{{10, 10, 1}, {20, 20, 1}, {30, 30, 1}},
			want:  []Range{{10, 30, 10}},
		},
		{
			name:  "adjacent_stepped_same_step",
			input: []Range{{1, 9, 2}, {11, 19, 2}},
			want:  []Range{{1, 19, 2}},
		},
		{
			name:  "contiguous_plus_trailing_singles",
			input: []Range{{1, 5, 1}, {10, 10, 1}, {20, 20, 1}},
			want:  []Range{{1, 5, 1}, {10, 10, 1}, {20, 20, 1}},
		},
		{
			name: "contiguous_blocks_between_stepped",
			input: []Range{
				{1, 1, 1}, {3, 3, 1}, {5, 5, 1}, {7, 7, 1}, {9, 9, 1},
				{10, 11, 1},
			},
			// Frames: {1,3,5,7,9,10,11}. After mergeContiguous: 9 merges
			// with [10-11] → [9-11]. Singles: {1,3,5,7} → stepped [1-7:2].
			want: []Range{{1, 7, 2}, {9, 11, 1}},
		},
		{
			name:  "duplicate_frames_deduplicated",
			input: []Range{{1, 5, 1}, {3, 7, 1}},
			want:  []Range{{1, 7, 1}},
		},
		{
			name:  "single_frame",
			input: []Range{{42, 42, 1}},
			want:  []Range{{42, 42, 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Sequence{Ranges: tt.input}
			s.normalizeRanges()
			if !reflect.DeepEqual(s.Ranges, tt.want) {
				t.Errorf("normalizeRanges() = %v, want %v", s.Ranges, tt.want)
			}
			// Idempotency: normalizing again must produce the same result.
			before := make([]Range, len(s.Ranges))
			copy(before, s.Ranges)
			s.normalizeRanges()
			if !reflect.DeepEqual(s.Ranges, before) {
				t.Errorf("not idempotent: second normalize = %v, first = %v", s.Ranges, before)
			}
		})
	}
}

// TestNormalizePreservesFrames verifies that normalization never changes the
// set of frame numbers a sequence describes.
func TestNormalizePreservesFrames(t *testing.T) {
	cases := [][]Range{
		{{1, 100, 1}, {50, 150, 1}},
		{{1, 9, 2}, {2, 10, 2}},
		{{1, 1, 1}, {3, 3, 1}, {5, 5, 1}, {7, 7, 1}, {9, 9, 1}, {10, 11, 1}},
		{{100, 200, 3}, {1, 50, 1}},
		{{2, 2, 1}, {4, 4, 1}, {6, 6, 1}, {8, 8, 1}, {10, 10, 1}},
		{{1, 100, 1}, {101, 200, 2}},
	}

	for i, input := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			// Collect frames before normalization.
			s := &Sequence{Ranges: input}
			before := s.frames()

			s.normalizeRanges()
			after := s.frames()

			if !reflect.DeepEqual(before, after) {
				t.Errorf("frames changed: before=%d frames, after=%d frames", len(before), len(after))
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

	expanded, err := expandSequenceEntryWithManifest(entry, manifest)
	if err != nil {
		t.Fatalf("expandSequenceEntryWithManifest() error = %v", err)
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
// Sequence Escaping Tests
// ----------------------------------------------------------------------------

func TestUnescapeSequenceNotation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no escapes", "frame.", "frame."},
		{"backslash space", `my\ animation.`, "my animation."},
		{"backslash bracket", `file\[test\].`, "file[test]."},
		{"backslash backslash", `path\\to\\file.`, `path\to\file.`},
		{"backslash quote", `file\"v2\".`, `file"v2".`},
		{"backslash comma", `data\,backup.`, "data,backup."},
		{"backslash hyphen", `file\-name.`, "file-name."},
		{"multiple escapes", `my\ file\[v2\].`, "my file[v2]."},
		{"all escapes combined", `a\ b\[c\]d\\e\"f\,g\-h`, `a b[c]d\e"f,g-h`},
		{"trailing backslash", `file\`, `file\`},
		{"unknown escape", `file\x.`, `file\x.`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeSequenceNotation(tt.input)
			if got != tt.want {
				t.Errorf("unescapeSequenceNotation(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeSequenceNotation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no special chars", "frame.", "frame."},
		{"space", "my animation.", `my\ animation.`},
		{"brackets", "file[test].", `file\[test\].`},
		{"backslash", `path\to\file.`, `path\\to\\file.`},
		{"quote", `file"v2".`, `file\"v2\".`},
		{"comma not escaped", "data,backup.", "data,backup."},
		{"hyphen not escaped", "file-name.", "file-name."},
		{"multiple specials", "my file[v2].", `my\ file\[v2\].`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeSequenceNotation(tt.input)
			if got != tt.want {
				t.Errorf("escapeSequenceNotation(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSequenceEscaping(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		wantPrefix string
		wantSuffix string
		wantErr    bool
	}{
		{
			name:       "space in prefix",
			pattern:    `my\ animation.[001-100].exr`,
			wantPrefix: "my animation.",
			wantSuffix: ".exr",
		},
		{
			name:       "brackets in prefix",
			pattern:    `file\[test\].[001-010].dat`,
			wantPrefix: "file[test].",
			wantSuffix: ".dat",
		},
		{
			name:       "comma in prefix",
			pattern:    `data\,backup.[01-05].csv`,
			wantPrefix: "data,backup.",
			wantSuffix: ".csv",
		},
		{
			name:       "multiple escapes in prefix",
			pattern:    `my\ file\[v2\].[001-100].exr`,
			wantPrefix: "my file[v2].",
			wantSuffix: ".exr",
		},
		{
			name:       "backslash in prefix",
			pattern:    `path\\to\\file.[001-010].txt`,
			wantPrefix: `path\to\file.`,
			wantSuffix: ".txt",
		},
		{
			name:       "escape in suffix",
			pattern:    `frame.[001-100].my\ suffix`,
			wantPrefix: "frame.",
			wantSuffix: ".my suffix",
		},
		{
			name:       "quote in prefix",
			pattern:    `file\"v2\".[001-010].dat`,
			wantPrefix: `file"v2".`,
			wantSuffix: ".dat",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq, err := ParseSequence(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSequence(%q) error = %v, wantErr %v", tt.pattern, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if seq.Prefix != tt.wantPrefix {
				t.Errorf("Prefix = %q, want %q", seq.Prefix, tt.wantPrefix)
			}
			if seq.Suffix != tt.wantSuffix {
				t.Errorf("Suffix = %q, want %q", seq.Suffix, tt.wantSuffix)
			}
		})
	}
}

func TestFormatSequenceName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain sequence",
			in:   "frame.[0001-0100].exr",
			want: "frame.[0001-0100].exr",
		},
		{
			name: "space in prefix",
			in:   "my animation.[001-100].exr",
			want: `my\ animation.[001-100].exr`,
		},
		{
			name: "brackets in prefix",
			in:   "file[test].[001-010].dat",
			want: `file\[test\].[001-010].dat`,
		},
		{
			name: "backslash in prefix",
			in:   `path\to\file.[001-010].txt`,
			want: `path\\to\\file.[001-010].txt`,
		},
		{
			name: "quote in prefix",
			in:   `file"v2".[001-010].dat`,
			want: `file\"v2\".[001-010].dat`,
		},
		{
			name: "space in suffix",
			in:   "frame.[001-100].my suffix",
			want: `frame.[001-100].my\ suffix`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSequenceName(tt.in)
			if got != tt.want {
				t.Errorf("formatSequenceName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSequenceEscapingRoundTrip(t *testing.T) {
	// Test that sequence names with special characters survive encode→decode round-trip
	tests := []struct {
		name    string
		seqName string
	}{
		{"plain", "frame.[0001-0100].exr"},
		{"space in prefix", "my animation.[001-100].exr"},
		{"brackets in prefix", "file[test].[001-010].dat"},
		{"backslash in prefix", `path\to\file.[001-010].txt`},
		{"quote in prefix", `file"v2".[001-010].dat`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an entry with the sequence name
			entry := &Entry{
				Name:       tt.seqName,
				Mode:       0644,
				Timestamp:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:       1024,
				IsSequence: true,
				Pattern:    tt.seqName,
			}

			// Encode to canonical form
			m := NewManifest()
			m.AddEntry(entry)
			data, err := Marshal(m)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			// Decode back
			m2, err := Unmarshal(data)
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", string(data), err)
			}

			if len(m2.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m2.Entries))
			}
			got := m2.Entries[0]
			if got.Name != tt.seqName {
				t.Errorf("round-trip name = %q, want %q", got.Name, tt.seqName)
			}
			if !got.IsSequence {
				t.Errorf("round-trip entry not marked as sequence")
			}
		})
	}
}

func TestDecoderSequenceEscaping(t *testing.T) {
	// Test that the decoder correctly handles all escape sequences in unquoted names
	tests := []struct {
		name     string
		line     string
		wantName string
		wantSeq  bool
	}{
		{
			name:     "backslash space in sequence",
			line:     "-rw-r--r-- 2024-01-01T00:00:00Z 1024 my\\ animation.[001-100].exr",
			wantName: "my animation.[001-100].exr",
			wantSeq:  true,
		},
		{
			name:     "escaped brackets not sequence",
			line:     "-rw-r--r-- 2024-01-01T00:00:00Z 1024 file\\[123\\].dat",
			wantName: "file[123].dat",
			wantSeq:  false,
		},
		{
			name:     "escaped comma",
			line:     "-rw-r--r-- 2024-01-01T00:00:00Z 1024 data\\,backup.[01-05].csv",
			wantName: "data,backup.[01-05].csv",
			wantSeq:  true,
		},
		{
			name:     "escaped hyphen",
			line:     "-rw-r--r-- 2024-01-01T00:00:00Z 1024 file\\-name.[01-05].csv",
			wantName: "file-name.[01-05].csv",
			wantSeq:  true,
		},
		{
			name:     "escaped quote",
			line:     "-rw-r--r-- 2024-01-01T00:00:00Z 1024 file\\\"v2\\\".[001-010].dat",
			wantName: `file"v2".[001-010].dat`,
			wantSeq:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "@c4m 1.0\n" + tt.line + "\n"
			m, err := Unmarshal([]byte(input))
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if len(m.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Entries))
			}
			entry := m.Entries[0]
			if entry.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", entry.Name, tt.wantName)
			}
			if entry.IsSequence != tt.wantSeq {
				t.Errorf("IsSequence = %v, want %v", entry.IsSequence, tt.wantSeq)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// IDList Tests (merged from idlist_test.go)
// ----------------------------------------------------------------------------

func TestIDList(t *testing.T) {
	t.Run("newIDList creates empty list", func(t *testing.T) {
		list := newIDList()
		if list.Count() != 0 {
			t.Errorf("expected 0 items, got %d", list.Count())
		}
	})

	t.Run("Add and Get", func(t *testing.T) {
		list := newIDList()
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
		list := newIDList()
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
		list := newIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		c4id := list.ComputeC4ID()
		if c4id.IsNil() {
			t.Errorf("expected non-nil C4 ID")
		}

		// Same list should produce same C4 ID
		list2 := newIDList()
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
		list, err := parseIDListFromString(input)
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
		list, err := parseIDListFromString(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if list.Count() != 1 {
			t.Errorf("expected 1 item, got %d", list.Count())
		}
	})

	t.Run("invalid ID format", func(t *testing.T) {
		input := "not-a-valid-c4-id\n"
		_, err := parseIDListFromString(input)
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
		list := newIDList()
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
		list := newIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		wrongID := c4.Identify(strings.NewReader("wrong"))

		_, err := ParseDataBlock(wrongID, content)
		if err == nil {
			t.Errorf("expected error for hash mismatch")
		}
	})

	t.Run("CRLF rejected in data block", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		content := id1.String() + "\r\n"
		_, err := ParseDataBlock(c4.ID{}, content)
		if err == nil {
			t.Fatal("expected error for CRLF in data block")
		}
		if !strings.Contains(err.Error(), "CR") {
			t.Errorf("expected CR error, got: %v", err)
		}
	})

	t.Run("GetIDList from block", func(t *testing.T) {
		list := newIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		content := list.Canonical()
		listID := list.ComputeC4ID()

		block, err := ParseDataBlock(listID, content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		retrieved, err := block.getIDList()
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
		list := newIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		list.Add(id1)

		block := createDataBlockFromIDList(list)
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
	list := newIDList()
	id1 := c4.Identify(strings.NewReader("test1"))
	id2 := c4.Identify(strings.NewReader("test2"))
	list.Add(id1)
	list.Add(id2)

	block := createDataBlockFromIDList(list)

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
	list := newIDList()
	id1 := c4.Identify(strings.NewReader("test1"))
	id2 := c4.Identify(strings.NewReader("test2"))
	list.Add(id1)
	list.Add(id2)

	block := createDataBlockFromIDList(list)
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

		_, err := block.getIDList()
		if err == nil {
			t.Error("expected error for non-ID list block")
		}
		if !strings.Contains(err.Error(), "not an ID list") {
			t.Errorf("expected 'not an ID list' error, got: %v", err)
		}
	})

	t.Run("returns ID list for valid block", func(t *testing.T) {
		// Create a valid ID list block
		list := newIDList()
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))
		list.Add(id1)
		list.Add(id2)

		block := createDataBlockFromIDList(list)

		retrieved, err := block.getIDList()
		if err != nil {
			t.Fatalf("getIDList() error = %v", err)
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

func TestDetectSequences_ConvenienceFunction(t *testing.T) {
	// The package-level DetectSequences function should use minLength=3.
	// A sequence of exactly 3 files should be collapsed.
	manifest := NewManifest()
	for i := 1; i <= 3; i++ {
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("shot.%04d.exr", i),
			Size:      1024,
			Timestamp: time.Now(),
			Mode:      0644,
		})
	}

	result := DetectSequences(manifest)

	// Should collapse into a single sequence entry
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (collapsed sequence), got %d", len(result.Entries))
	}
	if !result.Entries[0].IsSequence {
		t.Error("expected entry to be a sequence")
	}
	if result.Entries[0].Pattern != "shot.[0001-0003].exr" {
		t.Errorf("pattern = %q, want %q", result.Entries[0].Pattern, "shot.[0001-0003].exr")
	}

	// A sequence of only 2 files should NOT be collapsed (minLength=3)
	manifest2 := NewManifest()
	for i := 1; i <= 2; i++ {
		manifest2.AddEntry(&Entry{
			Name:      fmt.Sprintf("clip.%04d.dpx", i),
			Size:      512,
			Timestamp: time.Now(),
			Mode:      0644,
		})
	}

	result2 := DetectSequences(manifest2)

	// Should keep both entries individually (not collapsed)
	if len(result2.Entries) != 2 {
		t.Errorf("expected 2 entries (not collapsed), got %d", len(result2.Entries))
	}
	for _, e := range result2.Entries {
		if e.IsSequence {
			t.Error("2-file group should not be collapsed with default minLength=3")
		}
	}
}

// TestSequenceC4ID_NotIDListHash verifies that a sequence's C4 ID is NOT
// simply the hash of the ID list (the old, incorrect behavior).
func TestSequenceC4ID_NotIDListHash(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	manifest := NewManifest()
	idList := newIDList()
	for i := 1; i <= 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		idList.Add(id)
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	result := DetectSequences(manifest)

	var seqEntry *Entry
	for _, e := range result.Entries {
		if e.IsSequence {
			seqEntry = e
			break
		}
	}
	if seqEntry == nil {
		t.Fatal("no sequence entry found")
	}

	// The sequence C4 ID must NOT equal the hash of the bare ID list
	idListC4ID := idList.ComputeC4ID()
	if seqEntry.C4ID == idListC4ID {
		t.Errorf("sequence C4 ID should NOT equal the ID list hash, got %s", seqEntry.C4ID)
	}
}

// TestSequenceC4ID_MatchesCanonicalManifest verifies that a sequence's C4 ID
// equals the C4 ID of a canonical manifest built from the member entries.
func TestSequenceC4ID_MatchesCanonicalManifest(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	manifest := NewManifest()
	var memberEntries []*Entry
	for i := 1; i <= 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		entry := &Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		}
		manifest.AddEntry(entry)
		memberEntries = append(memberEntries, entry)
	}

	result := DetectSequences(manifest)

	var seqEntry *Entry
	for _, e := range result.Entries {
		if e.IsSequence {
			seqEntry = e
			break
		}
	}
	if seqEntry == nil {
		t.Fatal("no sequence entry found")
	}

	// Build the expected manifest from member entries (same as directory C4 ID computation)
	expectedManifest := NewManifest()
	for _, e := range memberEntries {
		entryCopy := *e
		entryCopy.Depth = 0
		expectedManifest.AddEntry(&entryCopy)
	}
	expectedC4ID := expectedManifest.ComputeC4ID()

	if seqEntry.C4ID != expectedC4ID {
		t.Errorf("sequence C4 ID = %s, want %s (canonical manifest ID)", seqEntry.C4ID, expectedC4ID)
	}
}

// TestSequenceC4ID_TimestampAffectsID verifies that changing a member's
// timestamp (but not content) changes the sequence C4 ID.
func TestSequenceC4ID_TimestampAffectsID(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	altTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	makeManifest := func(ts time.Time) *Manifest {
		m := NewManifest()
		for i := 1; i <= 5; i++ {
			id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
			m.AddEntry(&Entry{
				Name:      fmt.Sprintf("frame.%04d.exr", i),
				Size:      1024,
				Timestamp: ts,
				Mode:      0644,
				C4ID:      id,
			})
		}
		return m
	}

	result1 := DetectSequences(makeManifest(baseTime))
	result2 := DetectSequences(makeManifest(altTime))

	var id1, id2 c4.ID
	for _, e := range result1.Entries {
		if e.IsSequence {
			id1 = e.C4ID
		}
	}
	for _, e := range result2.Entries {
		if e.IsSequence {
			id2 = e.C4ID
		}
	}

	if id1.IsNil() || id2.IsNil() {
		t.Fatal("sequence entries not found")
	}
	if id1 == id2 {
		t.Error("changing member timestamps should change the sequence C4 ID")
	}
}

// TestSequenceC4ID_NaturalSortOrder verifies that the sequence C4 ID
// computation uses natural sort order (frame.0001.exr before frame.0002.exr).
func TestSequenceC4ID_NaturalSortOrder(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Add entries in reverse order — detection should still produce the same C4 ID
	manifestForward := NewManifest()
	manifestReverse := NewManifest()

	for i := 1; i <= 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		entry := &Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		}
		manifestForward.AddEntry(entry)
	}
	for i := 5; i >= 1; i-- {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		entry := &Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		}
		manifestReverse.AddEntry(entry)
	}

	result1 := DetectSequences(manifestForward)
	result2 := DetectSequences(manifestReverse)

	var id1, id2 c4.ID
	for _, e := range result1.Entries {
		if e.IsSequence {
			id1 = e.C4ID
		}
	}
	for _, e := range result2.Entries {
		if e.IsSequence {
			id2 = e.C4ID
		}
	}

	if id1.IsNil() || id2.IsNil() {
		t.Fatal("sequence entries not found")
	}
	if id1 != id2 {
		t.Errorf("insertion order should not affect sequence C4 ID: forward=%s, reverse=%s", id1, id2)
	}
}

// TestSequenceC4ID_DataBlockPreserved verifies that the data block is still
// created and linked for round-tripping, even though the C4 ID computation changed.
func TestSequenceC4ID_DataBlockPreserved(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	manifest := NewManifest()
	for i := 1; i <= 3; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("shot.%04d.exr", i),
			Size:      512,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	result := DetectSequences(manifest)

	// Should have a data block
	if len(result.DataBlocks) != 1 {
		t.Fatalf("expected 1 data block, got %d", len(result.DataBlocks))
	}

	block := result.DataBlocks[0]
	if !block.IsIDList {
		t.Error("data block should be an ID list")
	}

	// The data block should contain 3 IDs
	list, err := block.getIDList()
	if err != nil {
		t.Fatalf("failed to get ID list from data block: %v", err)
	}
	if list.Count() != 3 {
		t.Errorf("expected 3 IDs in data block, got %d", list.Count())
	}

	// The sequence entry should have dataBlockID set (for expansion)
	var seqEntry *Entry
	for _, e := range result.Entries {
		if e.IsSequence {
			seqEntry = e
			break
		}
	}
	if seqEntry == nil {
		t.Fatal("no sequence entry found")
	}
	if seqEntry.dataBlockID.IsNil() {
		t.Error("sequence entry should have dataBlockID set")
	}
	if seqEntry.dataBlockID != block.ID {
		t.Errorf("dataBlockID = %s, want %s", seqEntry.dataBlockID, block.ID)
	}

	// Expansion should still work via dataBlockID
	expander := NewSequenceExpander(SequenceEmbedded)
	expanded, _, err := expander.ExpandManifest(result)
	if err != nil {
		t.Fatalf("ExpandManifest failed: %v", err)
	}

	// Should have sequence entry + 3 expanded entries
	if len(expanded.Entries) != 4 {
		t.Errorf("expected 4 entries after expansion, got %d", len(expanded.Entries))
	}
}
