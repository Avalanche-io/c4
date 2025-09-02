package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdaptiveColumnIntegration(t *testing.T) {
	t.Run("adaptive columns with varying file names", func(t *testing.T) {
		// Create test directory with files of varying name lengths
		testDir := t.TempDir()
		
		testFiles := []struct {
			path    string
			content string
		}{
			{"short.txt", "content"},
			{"medium_length_file.txt", "content"},
			{"very_long_filename_that_should_affect_columns.txt", "content"},
			{"subdir/nested.txt", "nested content"},
			{"subdir/another_very_long_nested_filename_here.txt", "more content"},
			{"subdir/deeply/nested/file.txt", "deep content"},
		}
		
		// Create directories
		os.MkdirAll(filepath.Join(testDir, "subdir", "deeply", "nested"), 0755)
		
		// Create files
		for _, tf := range testFiles {
			path := filepath.Join(testDir, tf.path)
			os.MkdirAll(filepath.Dir(path), 0755)
			if err := os.WriteFile(path, []byte(tf.content), 0644); err != nil {
				t.Fatalf("Failed to create test file %s: %v", tf.path, err)
			}
		}
		
		// Test with different delay settings
		delays := []time.Duration{
			0,                      // No delay - immediate output
			100 * time.Millisecond, // Short delay
			500 * time.Millisecond, // Standard delay
		}
		
		for _, delay := range delays {
			t.Run(strings.ReplaceAll(delay.String(), " ", "_"), func(t *testing.T) {
				var buf bytes.Buffer
				
				// Generate with adaptive columns
				gen := NewStreamingGenerator(&buf, true, delay)
				gen.computeC4IDs = true
				gen.includeHidden = false
				
				err := gen.GenerateFromPathStreaming(testDir)
				if err != nil {
					t.Fatalf("Failed to generate manifest: %v", err)
				}
				gen.Close()
				
				output := buf.String()
				lines := strings.Split(strings.TrimSpace(output), "\n")
				
				// Verify basic structure
				if !strings.HasPrefix(lines[0], "@c4m") {
					t.Errorf("Missing header in output")
				}
				
				// Check that long filenames are present
				foundLong := false
				foundNested := false
				for _, line := range lines {
					if strings.Contains(line, "very_long_filename_that_should_affect_columns.txt") {
						foundLong = true
					}
					if strings.Contains(line, "another_very_long_nested_filename_here.txt") {
						foundNested = true
						// Should be indented
						if !strings.HasPrefix(line, "  ") {
							t.Error("Nested file should be indented")
						}
					}
				}
				
				if !foundLong {
					t.Error("Long filename not found")
				}
				if !foundNested {
					t.Error("Nested long filename not found")
				}
				
				// Verify C4 IDs are aligned (if present)
				var c4Positions []int
				for _, line := range lines[1:] { // Skip header
					if idx := strings.Index(line, "c4"); idx > 0 {
						c4Positions = append(c4Positions, idx)
					}
				}
				
				// Check that C4 IDs at same depth have consistent positioning
				if len(c4Positions) > 1 {
					// Group by approximate position (within 5 chars)
					groups := make(map[int]int)
					for _, pos := range c4Positions {
						groupKey := (pos / 10) * 10
						groups[groupKey]++
					}
					
					// Most C4 IDs should be in the same group
					maxGroup := 0
					for _, count := range groups {
						if count > maxGroup {
							maxGroup = count
						}
					}
					
					if float64(maxGroup)/float64(len(c4Positions)) < 0.7 {
						t.Errorf("C4 IDs not well aligned: %v", c4Positions)
					}
				}
			})
		}
	})
	
	t.Run("column adjustment during streaming", func(t *testing.T) {
		// Create test scenario where a very long filename appears
		// after initial output has started
		testDir := t.TempDir()
		
		// Create files in alphabetical order
		testFiles := []struct {
			name    string
			content string
		}{
			{"aaa_short.txt", "content"},
			{"bbb_medium.txt", "content"},
			{"ccc_still_reasonable.txt", "content"},
			{"ddd_suddenly_an_extremely_long_filename_that_wasnt_anticipated_earlier.txt", "content"},
			{"eee_back_to_normal.txt", "content"},
		}
		
		for _, tf := range testFiles {
			path := filepath.Join(testDir, tf.name)
			if err := os.WriteFile(path, []byte(tf.content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}
		
		var buf bytes.Buffer
		
		// Use minimal delay to ensure output starts before all files are scanned
		gen := NewStreamingGenerator(&buf, true, 10*time.Millisecond)
		gen.computeC4IDs = true
		
		err := gen.GenerateFromPathStreaming(testDir)
		if err != nil {
			t.Fatalf("Failed to generate manifest: %v", err)
		}
		gen.Close()
		
		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		
		// Find the lines with our test files
		var shortLine, longLine, afterLine string
		for _, line := range lines {
			if strings.Contains(line, "aaa_short.txt") {
				shortLine = line
			} else if strings.Contains(line, "ddd_suddenly_an_extremely_long_filename") {
				longLine = line
			} else if strings.Contains(line, "eee_back_to_normal.txt") {
				afterLine = line
			}
		}
		
		if shortLine == "" || longLine == "" || afterLine == "" {
			t.Fatal("Couldn't find expected files in output")
		}
		
		// The column should have adjusted for the long filename
		// Files after the long one should use the new column
		if strings.Contains(longLine, "c4") && strings.Contains(afterLine, "c4") {
			longC4Pos := strings.Index(longLine, "c4")
			afterC4Pos := strings.Index(afterLine, "c4")
			
			// The C4 position after should accommodate the long filename
			if afterC4Pos < longC4Pos {
				t.Errorf("C4 column didn't adjust properly: after=%d, long=%d", 
					afterC4Pos, longC4Pos)
			}
		}
	})
}

func TestAdaptiveManifestMethod(t *testing.T) {
	t.Run("WritePrettyAdaptive method", func(t *testing.T) {
		// Create a manifest with various entry lengths
		manifest := &Manifest{
			Version: "1.0",
			Entries: []*Entry{
				{
					Mode:      0644,
					Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					Size:      100,
					Name:      "short.txt",
					Depth:     0,
				},
				{
					Mode:      0755,
					Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					Size:      4096,
					Name:      "directory/",
					Depth:     0,
				},
				{
					Mode:      0644,
					Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					Size:      500000,
					Name:      "very_long_filename_that_requires_column_adjustment.dat",
					Depth:     1,
				},
			},
		}
		
		var buf bytes.Buffer
		_, err := manifest.WritePrettyAdaptive(&buf, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("WritePrettyAdaptive failed: %v", err)
		}
		
		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		
		// Verify output
		if !strings.HasPrefix(lines[0], "@c4m 1.0") {
			t.Errorf("Invalid header: %s", lines[0])
		}
		
		// Check all entries are present
		if !strings.Contains(output, "short.txt") {
			t.Error("short.txt not found")
		}
		if !strings.Contains(output, "directory/") {
			t.Error("directory/ not found")
		}
		if !strings.Contains(output, "very_long_filename_that_requires_column_adjustment.dat") {
			t.Error("long filename not found")
		}
		
		// Verify the long filename is indented
		for _, line := range lines {
			if strings.Contains(line, "very_long_filename_that_requires_column_adjustment.dat") {
				if !strings.HasPrefix(line, "  ") {
					t.Error("Long filename should be indented")
				}
				break
			}
		}
	})
}