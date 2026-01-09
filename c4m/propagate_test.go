package c4m

import (
	"os"
	"testing"
	"time"
)

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
	now := time.Now().UTC()
	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 3, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		entries  []*Entry
		expected time.Time
		isNow    bool // if true, expect current time (can't compare exactly)
	}{
		{
			name:    "empty returns now",
			entries: []*Entry{},
			isNow:   true,
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
			name: "all null timestamps returns now",
			entries: []*Entry{
				{Name: "a.txt", Timestamp: time.Unix(0, 0)},
				{Name: "b.txt", Timestamp: time.Unix(0, 0)},
			},
			isNow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMostRecentModtime(tt.entries)
			if tt.isNow {
				// Check that result is close to now (within 1 second)
				diff := now.Sub(result)
				if diff < -time.Second || diff > time.Second {
					t.Errorf("expected time close to now, got %v (diff: %v)", result, diff)
				}
			} else {
				if !result.Equal(tt.expected) {
					t.Errorf("got %v, want %v", result, tt.expected)
				}
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
		PropagateMetadata(entries)

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
		PropagateMetadata(entries)

		if !dir.Timestamp.Equal(t2) {
			t.Errorf("dir timestamp: got %v, want %v", dir.Timestamp, t2)
		}
	})

	t.Run("propagates both size and timestamp", func(t *testing.T) {
		dir := &Entry{Name: "dir/", Mode: os.ModeDir, Size: -1, Timestamp: time.Unix(0, 0), Depth: 0}
		file1 := &Entry{Name: "a.txt", Size: 100, Timestamp: t1, Depth: 1}
		file2 := &Entry{Name: "b.txt", Size: 200, Timestamp: t2, Depth: 1}

		entries := []*Entry{dir, file1, file2}
		PropagateMetadata(entries)

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
		PropagateMetadata(entries)

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
		PropagateMetadata(entries)

		// subdir should get file1's info (its only direct child)
		if subdir.Size != 100 {
			t.Errorf("subdir size: got %d, want 100", subdir.Size)
		}
		if !subdir.Timestamp.Equal(t1) {
			t.Errorf("subdir timestamp: got %v, want %v", subdir.Timestamp, t1)
		}

		// root processes before subdir (single-pass), so subdir still has Size -1
		// calculateDirectorySize skips -1, so root only gets file2's size
		if root.Size != 200 {
			t.Errorf("root size: got %d, want 200", root.Size)
		}
		// root timestamp should be t2 (most recent of direct children: subdir has null, file2 has t2)
		if !root.Timestamp.Equal(t2) {
			t.Errorf("root timestamp: got %v, want %v", root.Timestamp, t2)
		}
	})

	t.Run("skips non-directory entries", func(t *testing.T) {
		file := &Entry{Name: "file.txt", Size: -1, Timestamp: time.Unix(0, 0), Depth: 0}
		entries := []*Entry{file}
		PropagateMetadata(entries)

		// File should not be modified (null values preserved)
		if file.Size != -1 {
			t.Errorf("file size should remain null: got %d", file.Size)
		}
	})

	t.Run("handles empty entries", func(t *testing.T) {
		entries := []*Entry{}
		PropagateMetadata(entries) // Should not panic
	})
}
