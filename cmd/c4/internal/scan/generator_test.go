package scan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestNewGenerator(t *testing.T) {
	g := NewGenerator()
	if g == nil {
		t.Fatal("NewGenerator() returned nil")
	}
	if !g.computeC4IDs {
		t.Error("computeC4IDs = false, want true")
	}
	if g.followSymlinks {
		t.Error("followSymlinks = true, want false")
	}
	if g.includeHidden {
		t.Error("includeHidden = true, want false")
	}
	if g.detectSequences {
		t.Error("detectSequences = true, want false")
	}
}

func TestGeneratorOptions(t *testing.T) {
	// Test WithC4IDs
	g1 := NewGenerator()
	WithC4IDs(false)(g1)
	if g1.computeC4IDs {
		t.Error("computeC4IDs = true, want false")
	}
	
	// Test WithSymlinks
	g2 := NewGenerator()
	WithSymlinks(true)(g2)
	if !g2.followSymlinks {
		t.Error("followSymlinks = false, want true")
	}
	
	// Test WithHidden
	g3 := NewGenerator()
	WithHidden(true)(g3)
	if !g3.includeHidden {
		t.Error("includeHidden = false, want true")
	}
	
	// Test WithSequenceDetection
	g4 := NewGenerator()
	WithSequenceDetection(false)(g4)
	if g4.detectSequences {
		t.Error("detectSequences = true, want false")
	}
}

func TestNewGeneratorWithOptions(t *testing.T) {
	g := NewGeneratorWithOptions(
		WithC4IDs(false),
		WithSymlinks(true),
		WithHidden(true),
		WithSequenceDetection(false),
	)
	
	if g.computeC4IDs {
		t.Error("computeC4IDs should be false")
	}
	if !g.followSymlinks {
		t.Error("followSymlinks should be true")
	}
	if !g.includeHidden {
		t.Error("includeHidden should be true")
	}
	if g.detectSequences {
		t.Error("detectSequences should be false")
	}
}

func TestGenerateFromPath(t *testing.T) {
	// Create test directory structure
	tmpdir, err := os.MkdirTemp("", "generator_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	
	// Create test files
	files := map[string]string{
		"file1.txt":           "content1",
		"file2.txt":           "content2",
		".hidden":             "hidden",
		"dir1/file3.txt":      "content3",
		"dir1/dir2/file4.txt": "content4",
	}
	
	for path, content := range files {
		fullPath := filepath.Join(tmpdir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	
	tests := []struct {
		name         string
		generator    *Generator
		checkEntries []string // Entries that should exist
		checkMissing []string // Entries that should not exist
	}{
		{
			name:      "default settings (no hidden files)",
			generator: NewGenerator(),
			checkEntries: []string{
				"file1.txt",
				"file2.txt",
				"dir1/",
				"dir1/file3.txt",
				"dir1/dir2/",
				"dir1/dir2/file4.txt",
			},
			checkMissing: []string{".hidden"},
		},
		{
			name:      "include hidden files",
			generator: NewGeneratorWithOptions(WithHidden(true)),
			checkEntries: []string{
				"file1.txt",
				"file2.txt",
				".hidden",
				"dir1/",
				"dir1/file3.txt",
				"dir1/dir2/",
				"dir1/dir2/file4.txt",
			},
			checkMissing: []string{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := tt.generator.GenerateFromPath(tmpdir)
			if err != nil {
				t.Fatalf("GenerateFromPath() error = %v", err)
			}
			
			// Build a map of entries by depth and name for verification
			entryMap := make(map[string]bool)
			for _, e := range manifest.Entries {
				key := fmt.Sprintf("depth%d:%s", e.Depth, e.Name)
				entryMap[key] = true
			}
			
			// Expected entries with their depths
			expected := map[string]int{
				"file1.txt": 0,
				"file2.txt": 0,
				"dir1/": 0,
				"file3.txt": 1,
				"dir2/": 1,
				"file4.txt": 2,
			}
			
			if tt.name == "include hidden files" {
				expected[".hidden"] = 0
			}
			
			// Check expected entries
			for name, depth := range expected {
				key := fmt.Sprintf("depth%d:%s", depth, name)
				if !entryMap[key] {
					t.Errorf("Expected entry %q at depth %d not found", name, depth)
				}
			}
			
			// Check missing entries
			if tt.name == "default settings (no hidden files)" {
				key := "depth0:.hidden"
				if entryMap[key] {
					t.Errorf("Unexpected hidden file found")
				}
			}
		})
	}
}

func TestGenerateFromPathWithC4IDs(t *testing.T) {
	// Create test directory structure
	tmpdir, err := os.MkdirTemp("", "generator_c4_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	
	// Create test files with known content
	testFile := filepath.Join(tmpdir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create subdirectory with file
	subdir := filepath.Join(tmpdir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	subFile := filepath.Join(subdir, "sub.txt")
	if err := os.WriteFile(subFile, []byte("sub content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Test with C4 IDs enabled (default)
	g1 := NewGenerator()
	manifest1, err := g1.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatalf("GenerateFromPath() error = %v", err)
	}
	
	// Check that C4 IDs were computed
	fileEntry := manifest1.GetEntry("test.txt")
	if fileEntry == nil {
		t.Fatal("test.txt entry not found")
	}
	if fileEntry.C4ID.IsNil() {
		t.Error("File C4 ID was not computed")
	}
	
	// Check that directory C4 ID was computed
	dirEntry := manifest1.GetEntry("subdir/")
	if dirEntry == nil {
		t.Fatal("subdir/ entry not found")
	}
	if dirEntry.C4ID.IsNil() {
		t.Error("Directory C4 ID was not computed")
	}
	
	// Test with C4 IDs disabled
	g2 := NewGeneratorWithOptions(WithC4IDs(false))
	manifest2, err := g2.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatalf("GenerateFromPath() error = %v", err)
	}
	
	fileEntry2 := manifest2.GetEntry("test.txt")
	if fileEntry2 == nil {
		t.Fatal("test.txt entry not found")
	}
	if !fileEntry2.C4ID.IsNil() {
		t.Error("File C4 ID was computed when disabled")
	}
}

func TestGenerateFromPathErrors(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "non-existent path",
			path:    "/non/existent/path",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: false, // Will resolve to current directory
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGenerator()
			_, err := g.GenerateFromPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateFromPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateFromPathSymlinks(t *testing.T) {
	// Skip on systems that don't support symlinks well
	if os.Getenv("CI") != "" {
		t.Skip("Skipping symlink test in CI environment")
	}
	
	// Create test directory with symlinks
	tmpdir, err := os.MkdirTemp("", "generator_symlink_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	
	// Create a regular file
	targetFile := filepath.Join(tmpdir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create a symlink to the file
	linkFile := filepath.Join(tmpdir, "link.txt")
	if err := os.Symlink("target.txt", linkFile); err != nil {
		t.Skip("Cannot create symlinks on this system")
	}
	
	// Create a directory
	targetDir := filepath.Join(tmpdir, "targetdir")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	
	// Create a symlink to the directory
	linkDir := filepath.Join(tmpdir, "linkdir")
	if err := os.Symlink("targetdir", linkDir); err != nil {
		t.Fatal(err)
	}
	
	g := NewGenerator()
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatalf("GenerateFromPath() error = %v", err)
	}
	
	// Check symlink to file
	linkEntry := manifest.GetEntry("link.txt")
	if linkEntry == nil {
		t.Fatal("link.txt entry not found")
	}
	if !linkEntry.IsSymlink() {
		t.Error("link.txt should be a symlink")
	}
	if linkEntry.Target != "target.txt" {
		t.Errorf("link.txt target = %q, want %q", linkEntry.Target, "target.txt")
	}
	
	// Check symlink to directory
	linkDirEntry := manifest.GetEntry("linkdir")
	if linkDirEntry == nil {
		t.Fatal("linkdir entry not found")
	}
	if !linkDirEntry.IsSymlink() {
		t.Error("linkdir should be a symlink")
	}
	if linkDirEntry.Target != "targetdir" {
		t.Errorf("linkdir target = %q, want %q", linkDirEntry.Target, "targetdir")
	}
}

func TestGenerateFromPathSingleFile(t *testing.T) {
	// Create a single test file
	tmpdir, err := os.MkdirTemp("", "generator_single_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	
	testFile := filepath.Join(tmpdir, "single.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Generate manifest from single file
	g := NewGenerator()
	manifest, err := g.GenerateFromPath(testFile)
	if err != nil {
		t.Fatalf("GenerateFromPath() error = %v", err)
	}
	
	// Should have exactly one entry with just the filename
	if len(manifest.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(manifest.Entries))
	}
	
	// The entry name should be just the basename
	entry := manifest.Entries[0]
	if entry.Name != "single.txt" {
		t.Errorf("Entry name = %q, want %q", entry.Name, "single.txt")
	}
}

func TestGroupSequences(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantSeq  bool
		wantName string
	}{
		{
			name:     "sequence of files",
			files:    []string{"frame001.exr", "frame002.exr", "frame003.exr"},
			wantSeq:  true,
			wantName: "frame[001-003].exr",
		},
		{
			name:     "non-sequential files",
			files:    []string{"file1.txt", "other.txt", "readme.md"},
			wantSeq:  false,
		},
		{
			name:     "single file",
			files:    []string{"single.txt"},
			wantSeq:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpdir, err := os.MkdirTemp("", "seq_test")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpdir)

			// Create test files
			for _, name := range tt.files {
				file := filepath.Join(tmpdir, name)
				if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Generate manifest with sequence detection
			g := NewGeneratorWithOptions(WithSequenceDetection(true))
			manifest, err := g.GenerateFromPath(tmpdir)
			if err != nil {
				t.Fatal(err)
			}

			if tt.wantSeq {
				// Should have 1 sequence entry
				if len(manifest.Entries) != 1 {
					t.Errorf("Expected 1 sequence entry, got %d", len(manifest.Entries))
				}
				if manifest.Entries[0].Name != tt.wantName {
					t.Errorf("Expected sequence name %q, got %q", tt.wantName, manifest.Entries[0].Name)
				}
				if !manifest.Entries[0].IsSequence {
					t.Error("Entry should be marked as sequence")
				}
			} else {
				// Should have individual entries
				if len(manifest.Entries) != len(tt.files) {
					t.Errorf("Expected %d entries, got %d", len(tt.files), len(manifest.Entries))
				}
				for _, e := range manifest.Entries {
					if e.IsSequence {
						t.Error("Entry should not be marked as sequence")
					}
				}
			}
		})
	}
}

func TestSymlinkC4IDs(t *testing.T) {
	// Skip on systems that don't support symlinks well
	if os.Getenv("CI") != "" {
		t.Skip("Skipping symlink test in CI environment")
	}

	tmpdir, err := os.MkdirTemp("", "symlink_c4id_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Create test files with known content
	targetFile := filepath.Join(tmpdir, "target.txt")
	content := []byte("hello world")
	if err := os.WriteFile(targetFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute expected C4 ID for the content
	expectedID := c4.Identify(bytes.NewReader(content))

	// Create symlink to file
	linkToFile := filepath.Join(tmpdir, "link_to_file.txt")
	if err := os.Symlink("target.txt", linkToFile); err != nil {
		t.Skip("Cannot create symlinks on this system")
	}

	// Create symlink to symlink
	linkToLink := filepath.Join(tmpdir, "link_to_link.txt")
	if err := os.Symlink("link_to_file.txt", linkToLink); err != nil {
		t.Fatal(err)
	}

	// Create broken symlink
	brokenLink := filepath.Join(tmpdir, "broken_link.txt")
	if err := os.Symlink("nonexistent.txt", brokenLink); err != nil {
		t.Fatal(err)
	}

	// Create directory with file
	targetDir := filepath.Join(tmpdir, "targetdir")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	dirFile := filepath.Join(targetDir, "file.txt")
	if err := os.WriteFile(dirFile, []byte("dir content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink to directory
	linkToDir := filepath.Join(tmpdir, "link_to_dir")
	if err := os.Symlink("targetdir", linkToDir); err != nil {
		t.Fatal(err)
	}

	// Generate manifest with C4 IDs
	g := NewGenerator() // computeC4IDs is true by default
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatalf("GenerateFromPath() error = %v", err)
	}

	tests := []struct {
		name       string
		entryName  string
		checkC4ID  func(t *testing.T, entry *Entry)
	}{
		{
			name:      "symlink to file has target's C4 ID",
			entryName: "link_to_file.txt",
			checkC4ID: func(t *testing.T, entry *Entry) {
				if !entry.IsSymlink() {
					t.Error("Entry should be a symlink")
				}
				if entry.C4ID.IsNil() {
					t.Error("Symlink to file should have C4 ID")
				}
				if entry.C4ID.String() != expectedID.String() {
					t.Errorf("C4 ID = %s, want %s", entry.C4ID, expectedID)
				}
			},
		},
		{
			name:      "symlink to symlink has empty C4 ID",
			entryName: "link_to_link.txt",
			checkC4ID: func(t *testing.T, entry *Entry) {
				if !entry.IsSymlink() {
					t.Error("Entry should be a symlink")
				}
				if !entry.C4ID.IsNil() {
					t.Error("Symlink to symlink should have empty C4 ID")
				}
			},
		},
		{
			name:      "broken symlink has empty C4 ID",
			entryName: "broken_link.txt",
			checkC4ID: func(t *testing.T, entry *Entry) {
				if !entry.IsSymlink() {
					t.Error("Entry should be a symlink")
				}
				if !entry.C4ID.IsNil() {
					t.Error("Broken symlink should have empty C4 ID")
				}
			},
		},
		{
			name:      "symlink to directory has directory's manifest C4 ID",
			entryName: "link_to_dir",
			checkC4ID: func(t *testing.T, entry *Entry) {
				if !entry.IsSymlink() {
					t.Error("Entry should be a symlink")
				}
				if entry.C4ID.IsNil() {
					t.Error("Symlink to directory should have C4 ID")
				}
				// Should match the directory's manifest C4 ID
				dirEntry := manifest.GetEntry("targetdir/")
				if dirEntry != nil && !dirEntry.C4ID.IsNil() {
					if entry.C4ID.String() != dirEntry.C4ID.String() {
						t.Errorf("Symlink C4 ID should match directory C4 ID")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := manifest.GetEntry(tt.entryName)
			if entry == nil {
				t.Fatalf("Entry %q not found", tt.entryName)
			}
			tt.checkC4ID(t, entry)
		})
	}
}

func TestGenerateFromPathSymlinkEdgeCases(t *testing.T) {
	// Skip on systems that don't support symlinks well
	if os.Getenv("CI") != "" {
		t.Skip("Skipping symlink test in CI environment")
	}

	tests := []struct {
		name     string
		setup    func(tmpdir string) error
		options  []GeneratorOption
		validate func(t *testing.T, manifest *Manifest)
	}{
		{
			name: "broken symlink",
			setup: func(tmpdir string) error {
				// Create a broken symlink (target doesn't exist)
				linkFile := filepath.Join(tmpdir, "broken.txt")
				return os.Symlink("nonexistent.txt", linkFile)
			},
			validate: func(t *testing.T, manifest *Manifest) {
				entry := manifest.GetEntry("broken.txt")
				if entry == nil {
					t.Fatal("broken.txt entry not found")
				}
				if !entry.IsSymlink() {
					t.Error("broken.txt should be a symlink")
				}
				if entry.Target != "nonexistent.txt" {
					t.Errorf("broken.txt target = %q, want %q", entry.Target, "nonexistent.txt")
				}
			},
		},
		{
			name: "absolute path symlink",
			setup: func(tmpdir string) error {
				// Create a file
				targetFile := filepath.Join(tmpdir, "target.txt")
				if err := os.WriteFile(targetFile, []byte("content"), 0644); err != nil {
					return err
				}
				// Create symlink with absolute path
				linkFile := filepath.Join(tmpdir, "abs_link.txt")
				return os.Symlink(targetFile, linkFile)
			},
			validate: func(t *testing.T, manifest *Manifest) {
				entry := manifest.GetEntry("abs_link.txt")
				if entry == nil {
					t.Fatal("abs_link.txt entry not found")
				}
				if !entry.IsSymlink() {
					t.Error("abs_link.txt should be a symlink")
				}
				// Target should be preserved as-is
				if entry.Target == "" {
					t.Error("abs_link.txt target should not be empty")
				}
			},
		},
		{
			name: "nested symlink",
			setup: func(tmpdir string) error {
				// Create nested directory structure
				dir1 := filepath.Join(tmpdir, "dir1")
				dir2 := filepath.Join(dir1, "dir2")
				if err := os.MkdirAll(dir2, 0755); err != nil {
					return err
				}
				// Create file in dir2
				targetFile := filepath.Join(dir2, "target.txt")
				if err := os.WriteFile(targetFile, []byte("nested"), 0644); err != nil {
					return err
				}
				// Create symlink in dir1 pointing to file in dir2
				linkFile := filepath.Join(dir1, "link.txt")
				return os.Symlink("dir2/target.txt", linkFile)
			},
			validate: func(t *testing.T, manifest *Manifest) {
				// Find link.txt at depth 1
				var entry *Entry
				for _, e := range manifest.Entries {
					if e.Name == "link.txt" && e.Depth == 1 {
						entry = e
						break
					}
				}
				if entry == nil {
					t.Fatal("link.txt entry at depth 1 not found")
				}
				if !entry.IsSymlink() {
					t.Error("link.txt should be a symlink")
				}
				if entry.Target != "dir2/target.txt" {
					t.Errorf("link.txt target = %q, want %q", entry.Target, "dir2/target.txt")
				}
			},
		},
		{
			name: "follow symlinks option",
			setup: func(tmpdir string) error {
				// Create a file
				targetFile := filepath.Join(tmpdir, "target.txt")
				if err := os.WriteFile(targetFile, []byte("content"), 0644); err != nil {
					return err
				}
				// Create symlink
				linkFile := filepath.Join(tmpdir, "link.txt")
				return os.Symlink("target.txt", linkFile)
			},
			options: []GeneratorOption{WithSymlinks(true)},
			validate: func(t *testing.T, manifest *Manifest) {
				// When following symlinks, the link should appear as a regular file
				entry := manifest.GetEntry("link.txt")
				if entry == nil {
					t.Fatal("link.txt entry not found")
				}
				// When following symlinks, it should not be marked as symlink
				if entry.IsSymlink() {
					t.Error("link.txt should not be marked as symlink when following")
				}
				if entry.Target != "" {
					t.Error("link.txt should not have a target when following symlinks")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpdir, err := os.MkdirTemp("", "symlink_edge_test")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpdir)

			if err := tt.setup(tmpdir); err != nil {
				t.Skip("Cannot create symlinks on this system")
			}

			g := NewGeneratorWithOptions(tt.options...)
			manifest, err := g.GenerateFromPath(tmpdir)
			if err != nil {
				t.Fatalf("GenerateFromPath() error = %v", err)
			}

			tt.validate(t, manifest)
		})
	}
}

func TestSymlinkRangeGrouping(t *testing.T) {
	// Skip on systems that don't support symlinks well
	if os.Getenv("CI") != "" {
		t.Skip("Skipping symlink test in CI environment")
	}

	tests := []struct {
		name        string
		setup       func(tmpdir string) error
		wantEntries []string
		checkEntry  func(t *testing.T, entry *Entry)
	}{
		{
			name: "uniform symlink targets",
			setup: func(tmpdir string) error {
				// Create target files
				for i := 1; i <= 3; i++ {
					target := filepath.Join(tmpdir, fmt.Sprintf("source%03d.exr", i))
					if err := os.WriteFile(target, []byte(fmt.Sprintf("frame %d", i)), 0644); err != nil {
						return err
					}
				}
				// Create symlinks with uniform pattern
				for i := 1; i <= 3; i++ {
					link := filepath.Join(tmpdir, fmt.Sprintf("render%03d.exr", i))
					target := fmt.Sprintf("source%03d.exr", i)
					if err := os.Symlink(target, link); err != nil {
						return err
					}
				}
				return nil
			},
			wantEntries: []string{"render[001-003].exr"},
			checkEntry: func(t *testing.T, entry *Entry) {
				if !entry.IsSequence {
					t.Error("Should be a sequence")
				}
				if !entry.IsSymlink() {
					t.Error("Should be a symlink")
				}
				if entry.Target != "source[001-003].exr" {
					t.Errorf("Target = %q, want source[001-003].exr", entry.Target)
				}
			},
		},
		{
			name: "mixed symlink targets",
			setup: func(tmpdir string) error {
				// Create symlinks with different target patterns
				targets := []string{"cache/tmp001.exr", "final/beauty002.exr", "../shared/render003.exr"}
				for i, target := range targets {
					link := filepath.Join(tmpdir, fmt.Sprintf("render%03d.exr", i+1))
					if err := os.Symlink(target, link); err != nil {
						return err
					}
				}
				return nil
			},
			wantEntries: []string{"render[001-003].exr"},
			checkEntry: func(t *testing.T, entry *Entry) {
				if !entry.IsSequence {
					t.Error("Should be a sequence")
				}
				if !entry.IsSymlink() {
					t.Error("Should be a symlink")
				}
				if entry.Target != "..." {
					t.Errorf("Target = %q, want ...", entry.Target)
				}
			},
		},
		{
			name: "symlinks and regular files not grouped together",
			setup: func(tmpdir string) error {
				// Create some regular files
				for i := 1; i <= 2; i++ {
					file := filepath.Join(tmpdir, fmt.Sprintf("frame%03d.exr", i))
					if err := os.WriteFile(file, []byte(fmt.Sprintf("content %d", i)), 0644); err != nil {
						return err
					}
				}
				// Create some symlinks with same naming pattern
				for i := 3; i <= 4; i++ {
					link := filepath.Join(tmpdir, fmt.Sprintf("frame%03d.exr", i))
					if err := os.Symlink(fmt.Sprintf("source%03d.exr", i), link); err != nil {
						return err
					}
				}
				return nil
			},
			wantEntries: []string{"frame[001-002].exr", "frame[003-004].exr"},
			checkEntry: func(t *testing.T, entry *Entry) {
				// Should have two separate sequences
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpdir, err := os.MkdirTemp("", "symlink_range_test")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpdir)

			if err := tt.setup(tmpdir); err != nil {
				t.Skip("Cannot create symlinks on this system")
			}

			g := NewGeneratorWithOptions(WithSequenceDetection(true))
			manifest, err := g.GenerateFromPath(tmpdir)
			if err != nil {
				t.Fatalf("GenerateFromPath() error = %v", err)
			}

			// Check that we have the expected sequence entries
			var foundEntries []string
			for _, entry := range manifest.Entries {
				if entry.IsSequence && (tt.name == "symlinks and regular files not grouped together" || entry.IsSymlink()) {
					foundEntries = append(foundEntries, entry.Name)
				}
			}

			if len(foundEntries) != len(tt.wantEntries) {
				t.Errorf("Found %d sequences, want %d", len(foundEntries), len(tt.wantEntries))
				t.Logf("Found sequences: %v", foundEntries)
				for _, e := range manifest.Entries {
					if e.IsSequence {
						t.Logf("Entry: %s (IsSequence=%v, IsSymlink=%v, Target=%q)", e.Name, e.IsSequence, e.IsSymlink(), e.Target)
					}
				}
			}

			// Check specific entry if only one expected
			if len(tt.wantEntries) == 1 && tt.checkEntry != nil {
				entry := manifest.GetEntry(tt.wantEntries[0])
				if entry != nil {
					tt.checkEntry(t, entry)
				} else {
					t.Errorf("Expected entry %q not found", tt.wantEntries[0])
				}
			}
		})
	}
}

func TestGenerateFromReader(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, m *Manifest)
	}{
		{
			name: "valid manifest",
			input: `@c4m 1.0
-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt`,
			check: func(t *testing.T, m *Manifest) {
				if m.Version != "1.0" {
					t.Errorf("Version = %q, want 1.0", m.Version)
				}
				if len(m.Entries) != 1 {
					t.Errorf("Entries = %d, want 1", len(m.Entries))
				}
			},
		},
		{
			name:    "invalid manifest",
			input:   "not a valid manifest",
			wantErr: true,
		},
		{
			name: "empty manifest",
			input: `@c4m 1.0`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 0 {
					t.Errorf("Entries = %d, want 0", len(m.Entries))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateFromReader(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateFromReader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
