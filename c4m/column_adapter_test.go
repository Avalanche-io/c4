package c4m

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	
	"github.com/Avalanche-io/c4"
)

func TestColumnAdapter(t *testing.T) {
	t.Run("basic column calculation", func(t *testing.T) {
		adapter := NewColumnAdapter(100 * time.Millisecond)
		adapter.Start()
		defer adapter.Stop()
		
		// Initial column should be 80
		col := adapter.GetColumn()
		if col != 80 {
			t.Errorf("Initial column = %d, want 80", col)
		}
		
		// Simulate a long line
		entry := &Entry{
			Mode:      0644,
			Timestamp: time.Now(),
			Size:      1234567,
			Name:      "very_long_filename_that_might_cause_column_adjustment.txt",
			Depth:     0,
		}
		
		adapter.ScanEntry(entry)
		time.Sleep(10 * time.Millisecond) // Let scanner process
		
		// Force update with long line
		adapter.ForceUpdateColumn(100)
		
		// Column should have moved right
		newCol := adapter.GetColumn()
		if newCol <= 80 {
			t.Errorf("Column didn't move right: %d", newCol)
		}
		
		// Should be on a 10-column boundary
		if newCol%10 != 0 {
			t.Errorf("Column not on 10-boundary: %d", newCol)
		}
	})
	
	t.Run("column never moves left", func(t *testing.T) {
		adapter := NewColumnAdapter(0)
		adapter.Start()
		defer adapter.Stop()
		
		// Force a wide column
		adapter.ForceUpdateColumn(95)
		col1 := adapter.GetColumn()
		
		// Try to force a narrower column
		adapter.ForceUpdateColumn(70)
		col2 := adapter.GetColumn()
		
		// Column should not have moved left
		if col2 < col1 {
			t.Errorf("Column moved left: %d -> %d", col1, col2)
		}
	})
	
	t.Run("initial delay respected", func(t *testing.T) {
		delay := 200 * time.Millisecond
		adapter := NewColumnAdapter(delay)
		adapter.Start()
		defer adapter.Stop()
		
		start := time.Now()
		
		// Submit entries that would require wider column
		for i := 0; i < 10; i++ {
			adapter.ScanEntry(&Entry{
				Name: strings.Repeat("x", 100),
				Size: int64(i * 1000000),
			})
		}
		
		// First GetColumn call should wait for delay
		_ = adapter.GetColumn()
		elapsed := time.Since(start)
		
		// Should have waited close to the delay
		if elapsed < delay-50*time.Millisecond {
			t.Errorf("Didn't wait long enough: %v < %v", elapsed, delay)
		}
		
		// Second call should be immediate
		start2 := time.Now()
		_ = adapter.GetColumn()
		elapsed2 := time.Since(start2)
		
		if elapsed2 > 10*time.Millisecond {
			t.Errorf("Second call wasn't immediate: %v", elapsed2)
		}
	})
}

func TestStreamingWriter(t *testing.T) {
	t.Run("streaming output with adaptive columns", func(t *testing.T) {
		var buf bytes.Buffer
		writer := NewStreamingWriter(&buf, true, 50*time.Millisecond)
		
		// Write header
		err := writer.WriteHeader("1.0")
		if err != nil {
			t.Fatalf("WriteHeader error: %v", err)
		}
		
		// Write entries with increasing name lengths
		entries := []*Entry{
			{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      100,
				Name:      "short.txt",
				Depth:     0,
			},
			{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      200,
				Name:      "medium_length_file.txt",
				Depth:     0,
			},
			{
				Mode:      0755,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      4096,
				Name:      "directory_with_long_name/",
				Depth:     0,
			},
			{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      300,
				Name:      "very_very_very_long_filename_that_should_push_the_column.txt",
				Depth:     1,
			},
		}
		
		for _, entry := range entries {
			if err := writer.WriteEntry(entry); err != nil {
				t.Fatalf("WriteEntry error: %v", err)
			}
		}
		
		writer.Close()
		
		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		
		// Check header
		if !strings.HasPrefix(lines[0], "@c4m 1.0") {
			t.Errorf("Invalid header: %s", lines[0])
		}
		
		// Check that entries are present
		if len(lines) != 5 { // header + 4 entries
			t.Errorf("Expected 5 lines, got %d", len(lines))
		}
		
		// Verify formatting is maintained
		for i, line := range lines[1:] {
			if !strings.Contains(line, entries[i].Name) {
				t.Errorf("Line %d missing entry name: %s", i+1, line)
			}
		}
	})
	
	t.Run("column adjustment during output", func(t *testing.T) {
		var buf bytes.Buffer
		writer := NewStreamingWriter(&buf, true, 0) // No initial delay
		
		writer.WriteHeader("1.0")
		
		// Write a short entry
		shortEntry := &Entry{
			Mode:      0644,
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Size:      100,
			Name:      "short.txt",
			C4ID:      c4.ID{}, // Will use a test ID
			Depth:     0,
		}
		writer.WriteEntry(shortEntry)
		
		// Write a very long entry that should trigger column adjustment
		longName := strings.Repeat("x", 100) + ".txt"
		longEntry := &Entry{
			Mode:      0644,
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Size:      200,
			Name:      longName,
			Depth:     0,
		}
		writer.WriteEntry(longEntry)
		
		// Write another short entry - should use new column
		shortEntry2 := &Entry{
			Mode:      0644,
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Size:      300,
			Name:      "another.txt",
			Depth:     0,
		}
		writer.WriteEntry(shortEntry2)
		
		writer.Close()
		
		output := buf.String()
		if !strings.Contains(output, longName) {
			t.Errorf("Long filename not found in output")
		}
	})
}

func TestStreamingGenerator(t *testing.T) {
	t.Run("streaming generation", func(t *testing.T) {
		// Create a test directory structure
		testDir := t.TempDir()
		
		// Create some test files
		testFiles := []struct {
			path    string
			content string
			mode    os.FileMode
		}{
			{"file1.txt", "content1", 0644},
			{"file2.txt", "content2", 0644},
			{"subdir/file3.txt", "content3", 0644},
		}
		
		// Create subdir
		os.MkdirAll(testDir+"/subdir", 0755)
		
		for _, tf := range testFiles {
			path := testDir + "/" + tf.path
			err := os.WriteFile(path, []byte(tf.content), tf.mode)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}
		
		// Generate manifest with streaming
		var buf bytes.Buffer
		gen := NewStreamingGenerator(&buf, true, 100*time.Millisecond)
		gen.includeHidden = false
		gen.computeC4IDs = true
		
		err := gen.GenerateFromPathStreaming(testDir)
		if err != nil {
			t.Fatalf("GenerateFromPathStreaming error: %v", err)
		}
		
		gen.Close()
		
		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		
		// Verify header
		if !strings.HasPrefix(lines[0], "@c4m") {
			t.Errorf("Missing header: %s", lines[0])
		}
		
		// Verify files are present
		if !strings.Contains(output, "file1.txt") {
			t.Error("file1.txt not found")
		}
		if !strings.Contains(output, "file2.txt") {
			t.Error("file2.txt not found")
		}
		if !strings.Contains(output, "subdir/") {
			t.Error("subdir/ not found")
		}
		if !strings.Contains(output, "file3.txt") {
			t.Error("file3.txt not found")
		}
		
		// Check indentation for nested file
		for _, line := range lines {
			if strings.Contains(line, "file3.txt") {
				// Should have indentation
				if !strings.HasPrefix(line, "  ") {
					t.Error("file3.txt should be indented")
				}
				break
			}
		}
	})
}

func TestAdaptiveColumnPerformance(t *testing.T) {
	t.Run("performance with many entries", func(t *testing.T) {
		adapter := NewColumnAdapter(100 * time.Millisecond)
		adapter.Start()
		defer adapter.Stop()
		
		// Simulate scanning many entries
		start := time.Now()
		for i := 0; i < 1000; i++ {
			entry := &Entry{
				Mode:      0644,
				Timestamp: time.Now(),
				Size:      int64(i * 100),
				Name:      fmt.Sprintf("file_%04d.txt", i),
				Depth:     i % 5, // Vary depth
			}
			adapter.ScanEntry(entry)
		}
		
		// Get column should complete within initial delay + overhead
		col := adapter.GetColumn()
		elapsed := time.Since(start)
		
		if elapsed > 200*time.Millisecond {
			t.Errorf("Took too long to process entries: %v", elapsed)
		}
		
		// Column should be reasonable
		if col < 80 || col > 200 {
			t.Errorf("Unexpected column value: %d", col)
		}
	})
}