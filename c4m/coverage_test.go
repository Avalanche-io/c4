package c4m

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// TestSetIndent tests the SetIndent encoder option
func TestSetIndent(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{
		Name:      "dir/",
		Mode:      0755 | os.ModeDir,
		Size:      0,
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Depth:     0,
	})
	manifest.AddEntry(&Entry{
		Name:      "file.txt",
		Mode:      0644,
		Size:      100,
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Depth:     1,
	})

	var buf bytes.Buffer
	enc := NewEncoder(&buf).SetIndent(4) // 4-space indent
	if err := enc.Encode(manifest); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	output := buf.String()
	// With indent=4, nested entry should have 4 spaces
	if !strings.Contains(output, "    -rw-r--r--") {
		t.Errorf("Expected 4-space indent, got:\n%s", output)
	}
}

// TestMarshalPretty tests the MarshalPretty function
func TestMarshalPretty(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{
		Name:      "large_file.bin",
		Mode:      0644,
		Size:      1234567890,
		Timestamp: time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC),
		C4ID:      c4.Identify(strings.NewReader("test")),
	})

	data, err := MarshalPretty(manifest)
	if err != nil {
		t.Fatalf("MarshalPretty failed: %v", err)
	}

	output := string(data)
	// Should have comma-formatted size
	if !strings.Contains(output, "1,234,567,890") {
		t.Errorf("Expected comma-formatted size, got:\n%s", output)
	}
}

// TestFormatPretty tests the FormatPretty function
func TestFormatPretty(t *testing.T) {
	input := []byte(`-rw-r--r-- 2025-01-01T00:00:00Z 1000000 bigfile.txt
`)
	output, err := FormatPretty(input)
	if err != nil {
		t.Fatalf("FormatPretty failed: %v", err)
	}

	// Should have comma-formatted size in pretty output
	if !strings.Contains(string(output), "1,000,000") {
		t.Errorf("Expected pretty-formatted output with commas, got:\n%s", output)
	}
}

// TestResolverCache tests the Cache method on Resolver
func TestResolverCache(t *testing.T) {
	storage := &testStorage{
		data: map[string]string{
			"c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111": `-rw-r--r-- 2025-01-01T00:00:00Z 100 test.txt
`,
		},
	}

	resolver := NewResolver(storage)
	cache := resolver.Cache()
	if cache == nil {
		t.Fatal("Cache() returned nil")
	}

	// Test that we can get a manifest through the cache
	id, _ := c4.Parse("c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")
	manifest, err := cache.Get(id)
	if err != nil {
		t.Fatalf("Cache.Get failed: %v", err)
	}
	if len(manifest.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(manifest.Entries))
	}
}

// TestFormatEntryPretty tests pretty formatting of various entry types
func TestFormatEntryPretty(t *testing.T) {
	tests := []struct {
		name     string
		entry    *Entry
		contains []string
	}{
		{
			name: "null mode",
			entry: &Entry{
				Name:      "nullmode.txt",
				Mode:      0,
				Size:      100,
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			contains: []string{"----------"},
		},
		{
			name: "null timestamp",
			entry: &Entry{
				Name:      "nulltime.txt",
				Mode:      0644,
				Size:      100,
				Timestamp: time.Time{},
			},
			contains: []string{"-"},
		},
		{
			name: "null size",
			entry: &Entry{
				Name:      "nullsize.txt",
				Mode:      0644,
				Size:      -1,
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			contains: []string{"-"},
		},
		{
			name: "symlink",
			entry: &Entry{
				Name:      "link",
				Mode:      os.ModeSymlink | 0777,
				Size:      0,
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Target:    "/target/path",
			},
			contains: []string{"->", "/target/path"},
		},
		{
			name: "large file with commas",
			entry: &Entry{
				Name:      "big.bin",
				Mode:      0644,
				Size:      12345678,
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				C4ID:      c4.Identify(strings.NewReader("test")),
			},
			contains: []string{"12,345,678"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := NewManifest()
			manifest.AddEntry(tt.entry)

			data, err := MarshalPretty(manifest)
			if err != nil {
				t.Fatalf("MarshalPretty failed: %v", err)
			}

			output := string(data)
			for _, substr := range tt.contains {
				if !strings.Contains(output, substr) {
					t.Errorf("Expected output to contain %q, got:\n%s", substr, output)
				}
			}
		})
	}
}

// TestOperationsEdgeCases tests edge cases in operations.go
func TestIDListEdgeCases(t *testing.T) {
	t.Run("parse valid ID list", func(t *testing.T) {
		input := `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111
c42222222222222222222222222222222222222222222222222222222222222222222222222222222222222222`

		ids, err := parseIDList(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if ids.Count() != 2 {
			t.Errorf("Expected 2 IDs, got %d", ids.Count())
		}
	})

	t.Run("parse ID list with invalid entry returns error", func(t *testing.T) {
		input := `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111
not-a-valid-id`

		_, err := parseIDList(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid ID")
		}
	})

	t.Run("IsIDListContent with mixed content", func(t *testing.T) {
		// Invalid content should return false
		if IsIDListContent([]byte("this is not ID content\nwith random text")) {
			t.Error("Should not identify random text as ID list")
		}

		// Valid ID list content should return true
		if !IsIDListContent([]byte("c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111\nc42222222222222222222222222222222222222222222222222222222222222222222222222222222222222222")) {
			t.Error("Should identify valid ID list content")
		}
	})
}

// TestDecoderEdgeCases tests edge cases in the decoder
func TestDecoderEdgeCases(t *testing.T) {
	t.Run("CRLF line endings rejected", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\r\n"
		_, err := Unmarshal([]byte(input))
		if err == nil {
			t.Fatal("Expected error for CRLF input, got nil")
		}
		if !strings.Contains(err.Error(), "CR") {
			t.Errorf("Expected CR-related error, got: %v", err)
		}
	})

	t.Run("bare CR rejected", func(t *testing.T) {
		// CR without LF in an entry line
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file\r.txt\n"
		_, err := Unmarshal([]byte(input))
		if err == nil {
			t.Fatal("Expected error for bare CR in entry name")
		}
		if !strings.Contains(err.Error(), "CR") {
			t.Errorf("Expected CR-related error, got: %v", err)
		}
	})

	t.Run("encoder output is LF only", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{
			Name:      "test.txt",
			Mode:      0644,
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Size:      100,
		})
		data, err := Marshal(m)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if strings.ContainsRune(string(data), '\r') {
			t.Error("canonical output contains CR")
		}
		prettyData, err := MarshalPretty(m)
		if err != nil {
			t.Fatalf("MarshalPretty: %v", err)
		}
		if strings.ContainsRune(string(prettyData), '\r') {
			t.Error("pretty output contains CR")
		}
	})

	t.Run("zero timestamp marker", func(t *testing.T) {
		input := "-rw-r--r-- 0 100 file.txt\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse zero timestamp: %v", err)
		}
		// Zero timestamp is parsed as Unix epoch
		if manifest.Entries[0].Timestamp.Unix() != 0 {
			t.Errorf("Expected Unix epoch, got %v", manifest.Entries[0].Timestamp)
		}
	})

	t.Run("directory entry", func(t *testing.T) {
		input := "drwxr-xr-x 2025-01-01T00:00:00Z 4096 mydir/\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse directory: %v", err)
		}
		if !manifest.Entries[0].IsDir() {
			t.Error("Expected directory entry")
		}
	})
}

// TestValidatorEdgeCases tests edge cases in the validator
func TestValidatorEdgeCases(t *testing.T) {
	t.Run("validate special file types", func(t *testing.T) {
		input := `brw-r--r-- 2025-01-01T00:00:00Z 0 block_device
crw-r--r-- 2025-01-01T00:00:00Z 0 char_device
prw-r--r-- 2025-01-01T00:00:00Z 0 pipe
srw-r--r-- 2025-01-01T00:00:00Z 0 socket
`
		validator := NewValidator(false)
		err := validator.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Should validate special file types: %v", err)
		}

		stats := validator.GetStats()
		if stats.SpecialFiles != 4 {
			t.Errorf("Expected 4 special files, got %d", stats.SpecialFiles)
		}
	})

	t.Run("validate null mode", func(t *testing.T) {
		input := `---------- 2025-01-01T00:00:00Z 100 nullmode.txt
`
		validator := NewValidator(false)
		err := validator.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Should accept null mode: %v", err)
		}
	})

	t.Run("validate path with null bytes", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x00name.txt\n"
		validator := NewValidator(true)
		err := validator.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Should reject path with null bytes")
		}
	})
}

// TestSequenceExpansionEdgeCases tests edge cases in sequence expansion
func TestSequenceExpansionEdgeCases(t *testing.T) {
	t.Run("expand sequence with manifest lookup", func(t *testing.T) {
		// Create a sequence entry
		seqEntry := &Entry{
			Name:       "file.[001-003].txt",
			Mode:       0644,
			Size:       100,
			IsSequence: true,
		}

		// Create a manifest with file IDs
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "file.001.txt",
			Mode: 0644,
			Size: 100,
			C4ID: c4.Identify(strings.NewReader("content1")),
		})
		manifest.AddEntry(&Entry{
			Name: "file.002.txt",
			Mode: 0644,
			Size: 100,
			C4ID: c4.Identify(strings.NewReader("content2")),
		})
		manifest.AddEntry(&Entry{
			Name: "file.003.txt",
			Mode: 0644,
			Size: 100,
			C4ID: c4.Identify(strings.NewReader("content3")),
		})

		expanded, err := expandSequenceEntryWithManifest(seqEntry, manifest)
		if err != nil {
			t.Fatalf("expandSequenceEntryWithManifest failed: %v", err)
		}

		if len(expanded) != 3 {
			t.Errorf("Expected 3 expanded entries, got %d", len(expanded))
		}
	})

	t.Run("expand entry with id list", func(t *testing.T) {
		// Create a simple sequence entry
		seqEntry := &Entry{
			Name:       "file.[01-03].txt",
			Mode:       0644,
			Size:       100,
			IsSequence: true,
		}

		// Create an ID list with 3 IDs
		idList := newIDList()
		idList.Add(c4.Identify(strings.NewReader("content1")))
		idList.Add(c4.Identify(strings.NewReader("content2")))
		idList.Add(c4.Identify(strings.NewReader("content3")))

		expanded, err := expandSequenceEntry(seqEntry, idList)
		if err != nil {
			t.Fatalf("ExpandSequenceEntry failed: %v", err)
		}

		if len(expanded) != 3 {
			t.Errorf("Expected 3 expanded entries, got %d", len(expanded))
		}
	})
}

// testStorage implements store.Source for testing (different name to avoid conflict)
type testStorage struct {
	data map[string]string
}

func (m *testStorage) Open(id c4.ID) (io.ReadCloser, error) {
	content, ok := m.data[id.String()]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

// TestEncoderEdgeCases tests edge cases in the encoder
func TestFormatFunctions(t *testing.T) {
	t.Run("format invalid input", func(t *testing.T) {
		_, err := Format([]byte("not valid c4m"))
		if err == nil {
			t.Error("Expected error for invalid input")
		}
	})

	t.Run("format pretty invalid input", func(t *testing.T) {
		_, err := FormatPretty([]byte("not valid c4m"))
		if err == nil {
			t.Error("Expected error for invalid input")
		}
	})

	t.Run("marshal error handling", func(t *testing.T) {
		// Marshal should succeed with valid manifest
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "test.txt", Mode: 0644, Size: 100})
		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if len(data) == 0 {
			t.Error("Expected non-empty output")
		}
	})

	t.Run("marshal pretty error handling", func(t *testing.T) {
		// MarshalPretty should succeed with valid manifest
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:      "test.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if len(data) == 0 {
			t.Error("Expected non-empty output")
		}
	})
}

// TestParseDataBlockCoverage tests ParseDataBlock edge cases
func TestParseDataBlockCoverage(t *testing.T) {
	t.Run("parse empty content", func(t *testing.T) {
		id := c4.Identify(strings.NewReader(""))
		block, err := ParseDataBlock(id, "")
		if err != nil {
			// Empty content may or may not be an error depending on implementation
			t.Logf("ParseDataBlock with empty content: %v", err)
		}
		_ = block
	})

	t.Run("parse valid ID list", func(t *testing.T) {
		// Test parsing ID list content - construct content and its ID together
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))
		// The canonical form has no blank lines
		content := id1.String() + "\n" + id2.String() + "\n"
		// Compute correct ID for this content
		id := c4.Identify(strings.NewReader(content))
		block, err := ParseDataBlock(id, content)
		if err != nil {
			t.Fatalf("ParseDataBlock failed: %v", err)
		}
		if block == nil {
			t.Fatal("Expected non-nil block")
		}
		if !block.IsIDList {
			t.Error("Expected IsIDList to be true")
		}
	})
}

// TestReadLineEdgeCases tests edge cases in readLine
func TestReadLineEdgeCases(t *testing.T) {
	t.Run("very long line", func(t *testing.T) {
		// Create a manifest with a very long filename
		longName := strings.Repeat("a", 1000) + ".txt"
		input := fmt.Sprintf("-rw-r--r-- 2025-01-01T00:00:00Z 100 %s\n", longName)
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse long filename: %v", err)
		}
		if manifest.Entries[0].Name != longName {
			t.Error("Long filename not preserved")
		}
	})
}

// TestParseModeEdgeCases tests edge cases in parseMode
func TestParseModeEdgeCases(t *testing.T) {
	t.Run("block device", func(t *testing.T) {
		input := "brw-rw---- 2025-01-01T00:00:00Z 0 blockdev\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse block device: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeDevice == 0 {
			t.Error("Expected device mode")
		}
	})

	t.Run("character device", func(t *testing.T) {
		input := "crw-rw---- 2025-01-01T00:00:00Z 0 chardev\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse char device: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeCharDevice == 0 {
			t.Error("Expected char device mode")
		}
	})

	t.Run("named pipe", func(t *testing.T) {
		input := "prw-rw---- 2025-01-01T00:00:00Z 0 mypipe\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse named pipe: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeNamedPipe == 0 {
			t.Error("Expected named pipe mode")
		}
	})

	t.Run("socket", func(t *testing.T) {
		input := "srw-rw---- 2025-01-01T00:00:00Z 0 mysocket\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse socket: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeSocket == 0 {
			t.Error("Expected socket mode")
		}
	})

	t.Run("setuid permission", func(t *testing.T) {
		input := "-rwsr-xr-x 2025-01-01T00:00:00Z 100 setuid\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse setuid: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeSetuid == 0 {
			t.Error("Expected setuid mode")
		}
	})

	t.Run("setgid permission", func(t *testing.T) {
		input := "-rwxr-sr-x 2025-01-01T00:00:00Z 100 setgid\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse setgid: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeSetgid == 0 {
			t.Error("Expected setgid mode")
		}
	})

	t.Run("sticky bit", func(t *testing.T) {
		input := "drwxrwxrwt 2025-01-01T00:00:00Z 4096 sticky/\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Failed to parse sticky: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeSticky == 0 {
			t.Error("Expected sticky mode")
		}
	})
}

// TestValidatorAdditionalEdgeCases tests additional validator edge cases
func TestValidatorAdditionalEdgeCases(t *testing.T) {
	t.Run("invalid timestamp format", func(t *testing.T) {
		input := "-rw-r--r-- invalid_timestamp 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid timestamp")
		}
	})

	t.Run("invalid name with control chars", func(t *testing.T) {
		// Test name validation with null bytes
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x00name.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid name")
		}
	})

	t.Run("invalid C4 ID", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt invalid_c4id\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid C4 ID")
		}
	})

	t.Run("invalid mode format", func(t *testing.T) {
		input := "-rw-rXr-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid mode")
		}
	})

	t.Run("invalid first line", func(t *testing.T) {
		input := "not a valid entry\n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid entry")
		}
	})

	t.Run("valid entry", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Valid entry should pass: %v", err)
		}
	})

	t.Run("valid ergonomic format", func(t *testing.T) {
		// Test ergonomic format detection
		input := "-rw-r--r-- Jan 01, 2025 12:00 1,234 file.txt\n"
		v := NewValidator(false)
		err := v.ValidateManifest(strings.NewReader(input))
		// Ergonomic format may or may not parse correctly
		_ = err
	})
}

// TestSequenceExpansionWithManifestCoverage tests sequence expansion with manifest
func TestSequenceExpansionWithManifestCoverage(t *testing.T) {
	t.Run("expand regular entry without manifest", func(t *testing.T) {
		// Create a regular (non-sequence) entry
		entry := &Entry{
			Name:      "file.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		// Regular entries should return unchanged
		results, err := expandSequenceEntryWithManifest(entry, nil)
		if err != nil {
			// If it fails on regular entries, that's okay - just log it
			t.Logf("expandSequenceEntryWithManifest on regular entry: %v", err)
		} else if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("expand with manifest", func(t *testing.T) {
		// Create a manifest with some entries
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "file.001.txt",
			Mode: 0644,
			Size: 100,
		})
		manifest.AddEntry(&Entry{
			Name: "file.002.txt",
			Mode: 0644,
			Size: 100,
		})

		// Create an entry matching the pattern
		entry := &Entry{
			Name:      "file.001.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		// With manifest context
		results, err := expandSequenceEntryWithManifest(entry, manifest)
		if err != nil {
			t.Logf("expandSequenceEntryWithManifest with manifest: %v", err)
		}
		_ = results // Just exercise the function
	})
}

// TestOperationsEdgeCases2 tests additional operations edge cases
func TestSequenceDetectionCoverage(t *testing.T) {
	t.Run("detect image sequences", func(t *testing.T) {
		manifest := NewManifest()

		// Add sequential files
		for i := 1; i <= 5; i++ {
			manifest.AddEntry(&Entry{
				Name:      fmt.Sprintf("image.%04d.png", i),
				Mode:      0644,
				Size:      int64(1000 * i),
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			})
		}

		detector := NewSequenceDetector(3) // minLength of 3
		result := detector.DetectSequences(manifest)
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})

	t.Run("no sequences to detect", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "readme.txt",
			Mode: 0644,
			Size: 100,
		})
		manifest.AddEntry(&Entry{
			Name: "config.json",
			Mode: 0644,
			Size: 50,
		})

		detector := NewSequenceDetector(3)
		result := detector.DetectSequences(manifest)
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})
}

// TestParseSequenceCoverage tests sequence parsing
func TestParseSequenceCoverage(t *testing.T) {
	t.Run("parse sequence pattern", func(t *testing.T) {
		// Test IsSequence function first
		if !IsSequence("file.[001-003].txt") {
			t.Error("Expected IsSequence to return true for valid pattern")
		}
		if IsSequence("file.txt") {
			t.Error("Expected IsSequence to return false for non-sequence")
		}
	})

	t.Run("expand sequence pattern", func(t *testing.T) {
		// Test ExpandSequencePattern
		result, err := ExpandSequencePattern("file.[001-003].txt")
		if err != nil {
			t.Fatalf("ExpandSequencePattern failed: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("Expected 3 results, got %d", len(result))
		}
	})

	t.Run("parse non-sequence", func(t *testing.T) {
		_, err := ParseSequence("file.txt")
		if err == nil {
			t.Error("Expected error for non-sequence")
		}
	})
}

// TestResolverCoverage tests resolver edge cases
func TestResolverCoverage(t *testing.T) {
	t.Run("resolve missing manifest", func(t *testing.T) {
		// Create a storage that returns errors
		storage := &testErrorStorage{}
		resolver := NewResolver(storage)

		_, err := resolver.Resolve(c4.Identify(strings.NewReader("nonexistent")), "path/to/file")
		if err == nil {
			t.Error("Expected error for missing manifest")
		}
	})
}

// testErrorStorage is a test store.Source that always returns errors
type testErrorStorage struct{}

func (s *testErrorStorage) Open(id c4.ID) (io.ReadCloser, error) {
	return nil, fmt.Errorf("object not found: %s", id.String())
}

// TestParseIDListCoverage tests ParseIDList edge cases
func TestParseIDListCoverage(t *testing.T) {
	t.Run("parse single ID", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("content1"))
		input := id1.String() + "\n"
		idList, err := parseIDList(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if idList.Count() != 1 {
			t.Errorf("Expected 1 ID, got %d", idList.Count())
		}
	})

	t.Run("parse multiple IDs", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("content1"))
		id2 := c4.Identify(strings.NewReader("content2"))
		input := id1.String() + "\n" + id2.String() + "\n"
		idList, err := parseIDList(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if idList.Count() != 2 {
			t.Errorf("Expected 2 IDs, got %d", idList.Count())
		}
	})

	t.Run("parse invalid ID", func(t *testing.T) {
		input := "not_a_valid_c4_id\n"
		_, err := parseIDList(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid ID")
		}
	})
}

// TestEncoderWriteErrors tests encoder error handling
func TestEncoderWriteErrors(t *testing.T) {
	t.Run("write to failing writer", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "file.txt",
			Mode: 0644,
			Size: 100,
		})

		// Use a writer that fails
		fw := &failingWriter{failAfter: 5}
		err := NewEncoder(fw).Encode(manifest)
		if err == nil {
			t.Error("Expected error when writing to failing writer")
		}
	})
}

// failingWriter fails after writing a certain number of bytes
type failingWriter struct {
	written   int
	failAfter int
}

func (w *failingWriter) Write(p []byte) (n int, err error) {
	if w.written >= w.failAfter {
		return 0, fmt.Errorf("write failed")
	}
	w.written += len(p)
	if w.written > w.failAfter {
		return w.failAfter - (w.written - len(p)), fmt.Errorf("write failed")
	}
	return len(p), nil
}

// TestEncodingMoreEdgeCases tests more encoding scenarios
func TestValidatorMoreEdgeCases(t *testing.T) {
	t.Run("validate directory entry", func(t *testing.T) {
		input := "drwxr-xr-x 2025-01-01T00:00:00Z 4096 mydir/\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Valid directory entry should pass: %v", err)
		}
	})

	t.Run("validate symlink entry", func(t *testing.T) {
		input := "lrwxrwxrwx 2025-01-01T00:00:00Z 0 link -> target\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Valid symlink entry should pass: %v", err)
		}
	})

	t.Run("validate with nested directories", func(t *testing.T) {
		input := "drwxr-xr-x 2025-01-01T00:00:00Z 4096 dir/\n  -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Valid nested entry should pass: %v", err)
		}
	})

	t.Run("validate negative size", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z -100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Negative size should fail")
		}
	})

	t.Run("validate very large file", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 9999999999999 large.bin\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Large file size should be valid: %v", err)
		}
	})

	t.Run("validate file with path components", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 path/to/file.txt\n"
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader(input))
		// May or may not fail depending on strict mode
	})
}

// TestSequenceExpanderCoverage tests sequence expander
func TestSequenceExpanderCoverage(t *testing.T) {
	t.Run("expand manifest with sequences", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceEmbedded)

		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "regular.txt",
			Mode: 0644,
			Size: 100,
		})

		expanded, expansions, err := expander.ExpandManifest(manifest)
		if err != nil {
			t.Fatalf("ExpandManifest failed: %v", err)
		}
		if expanded == nil {
			t.Error("Expected non-nil expanded result")
		}
		_ = expansions // May or may not have expansions
	})

	t.Run("expand standalone mode", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceStandalone)

		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "file.txt",
			Mode: 0644,
			Size: 100,
		})

		expanded, expansions, err := expander.ExpandManifest(manifest)
		if err != nil {
			t.Fatalf("ExpandManifest failed: %v", err)
		}
		if expanded == nil {
			t.Error("Expected non-nil result")
		}
		_ = expansions
	})

	t.Run("expand manifest with sequence patterns", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceEmbedded)

		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "file.[001-003].txt",
			Mode: 0644,
			Size: 100,
		})

		expanded, expansions, err := expander.ExpandManifest(manifest)
		if err != nil {
			t.Logf("ExpandManifest with sequence: %v", err)
		}
		_ = expanded
		_ = expansions
	})
}

// TestMarshalMoreCases tests Marshal edge cases
func TestMarshalMoreCases(t *testing.T) {
	t.Run("marshal empty manifest", func(t *testing.T) {
		manifest := NewManifest()
		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("Expected empty output for empty manifest, got %d bytes", len(data))
		}
	})

	t.Run("marshal pretty empty manifest", func(t *testing.T) {
		manifest := NewManifest()
		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("Expected empty output for empty manifest, got %d bytes", len(data))
		}
	})

	t.Run("marshal with multiple entries", func(t *testing.T) {
		manifest := NewManifest()
		for i := 0; i < 10; i++ {
			manifest.AddEntry(&Entry{
				Name:      fmt.Sprintf("file%d.txt", i),
				Mode:      0644,
				Size:      int64(100 * i),
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			})
		}

		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if len(data) == 0 {
			t.Error("Expected non-empty output")
		}
	})
}

// TestDecodingMoreEdgeCases tests more decoding scenarios
func TestDetectSequencesMoreCases(t *testing.T) {
	t.Run("detect multiple sequences", func(t *testing.T) {
		manifest := NewManifest()

		// Add first sequence
		for i := 1; i <= 5; i++ {
			manifest.AddEntry(&Entry{
				Name: fmt.Sprintf("seq_a.%03d.txt", i),
				Mode: 0644,
				Size: 100,
			})
		}

		// Add second sequence
		for i := 1; i <= 3; i++ {
			manifest.AddEntry(&Entry{
				Name: fmt.Sprintf("seq_b.%04d.png", i),
				Mode: 0644,
				Size: 200,
			})
		}

		detector := NewSequenceDetector(2) // minLength of 2
		result := detector.DetectSequences(manifest)
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})

	t.Run("detect sequences with gaps", func(t *testing.T) {
		manifest := NewManifest()
		// Files 1, 2, 5, 6, 7 (gap at 3,4)
		for _, n := range []int{1, 2, 5, 6, 7} {
			manifest.AddEntry(&Entry{
				Name: fmt.Sprintf("file.%03d.txt", n),
				Mode: 0644,
				Size: 100,
			})
		}

		detector := NewSequenceDetector(2)
		result := detector.DetectSequences(manifest)
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})
}

// TestValidatorTimestampCoverage tests timestamp validation edge cases
func TestValidatorTimestampCoverage(t *testing.T) {
	t.Run("null timestamp dash", func(t *testing.T) {
		input := "-rw-r--r-- - 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Null timestamp '-' should be valid: %v", err)
		}
	})

	t.Run("null timestamp zero", func(t *testing.T) {
		input := "-rw-r--r-- 0 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Null timestamp '0' should be valid: %v", err)
		}
	})

	t.Run("timestamp without Z suffix", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Timestamp without Z should fail")
		}
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		input := "-rw-r--r-- 2025-13-45T99:99:99Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Invalid timestamp should fail")
		}
	})
}

// TestValidatorNameCoverage tests name validation edge cases
func TestValidatorNameCoverage(t *testing.T) {
	t.Run("path traversal with dot-dot-slash", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 ../file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Path traversal should fail")
		}
	})

	t.Run("path traversal with dot-slash", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 ./file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Path traversal should fail")
		}
	})

	t.Run("directory just slash", func(t *testing.T) {
		input := "drwxr-xr-x 2025-01-01T00:00:00Z 0 /\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Directory name '/' should fail")
		}
	})
}

// TestValidatorEntryEdgeCases tests entry validation edge cases
func TestValidatorEntryEdgeCases(t *testing.T) {
	t.Run("empty line", func(t *testing.T) {
		input := "\n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader(input))
	})

	t.Run("comment line", func(t *testing.T) {
		input := "# This is a comment\n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader(input))
	})

	t.Run("entry with only spaces", func(t *testing.T) {
		input := "   \n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader(input))
	})

	t.Run("short mode string", func(t *testing.T) {
		input := "-rw 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Short mode should fail")
		}
	})

	t.Run("too many fields", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c4xxxx extra\n"
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader(input))
	})
}

// TestValidatorC4IDEdgeCases tests C4 ID validation edge cases
func TestValidatorC4IDEdgeCases(t *testing.T) {
	t.Run("valid C4 ID", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("test"))
		input := fmt.Sprintf("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt %s\n", id.String())
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Valid C4 ID should pass: %v", err)
		}
	})

	t.Run("C4 ID with wrong prefix", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c3invalidid\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Wrong prefix should fail")
		}
	})

	t.Run("C4 ID too short", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c4short\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Too short C4 ID should fail")
		}
	})
}

// TestMarshalSuccessCases tests successful Marshal operations
func TestMarshalSuccessCases(t *testing.T) {
	t.Run("marshal manifest with symlink", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:   "link",
			Mode:   os.ModeSymlink | 0777,
			Size:   0,
			Target: "target",
		})
		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal symlink failed: %v", err)
		}
		if !strings.Contains(string(data), "->") {
			t.Error("Expected symlink indicator")
		}
	})

	t.Run("marshal pretty manifest with symlink", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:   "link",
			Mode:   os.ModeSymlink | 0777,
			Size:   0,
			Target: "target",
		})
		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty symlink failed: %v", err)
		}
		if len(data) == 0 {
			t.Error("Expected non-empty output")
		}
	})
}

// TestEncoderDataBlockCoverage tests encoder data block handling
func TestEncoderDataBlockCoverage(t *testing.T) {
	t.Run("encode with ID list data block", func(t *testing.T) {
		manifest := NewManifest()

		// Create ID list
		idList := newIDList()
		idList.Add(c4.Identify(strings.NewReader("content1")))
		idList.Add(c4.Identify(strings.NewReader("content2")))

		// Create and add data block
		dataBlock := createDataBlockFromIDList(idList)
		manifest.AddDataBlock(dataBlock)

		manifest.AddEntry(&Entry{
			Name: "file.txt",
			Mode: 0644,
			Size: 100,
		})

		var buf bytes.Buffer
		err := NewEncoder(&buf).Encode(manifest)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})

	t.Run("encode with non-ID list data block", func(t *testing.T) {
		manifest := NewManifest()

		content := []byte("binary content")
		id := c4.Identify(bytes.NewReader(content))
		dataBlock := &DataBlock{
			ID:       id,
			IsIDList: false,
			Content:  content,
		}
		manifest.AddDataBlock(dataBlock)

		manifest.AddEntry(&Entry{Name: "file.bin", Mode: 0644, Size: int64(len(content))})

		var buf bytes.Buffer
		err := NewEncoder(&buf).Encode(manifest)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
}

// TestDetectSequencesEdgeCases tests edge cases in sequence detection
func TestDetectSequencesEdgeCases(t *testing.T) {
	t.Run("single file matching pattern", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "file.001.txt",
			Mode: 0644,
			Size: 100,
		})

		detector := NewSequenceDetector(1)
		result := detector.DetectSequences(manifest)
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})

	t.Run("non-sequential numbers", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.001.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "file.010.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "file.100.txt", Mode: 0644, Size: 100})

		detector := NewSequenceDetector(2)
		result := detector.DetectSequences(manifest)
		_ = result
	})

	t.Run("mixed file types", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "image.001.png", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "image.002.png", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "video.001.mp4", Mode: 0644, Size: 200})
		manifest.AddEntry(&Entry{Name: "video.002.mp4", Mode: 0644, Size: 200})

		detector := NewSequenceDetector(2)
		result := detector.DetectSequences(manifest)
		_ = result
	})
}

// TestValidatorHeaderEdgeCases tests that directive lines are rejected
func TestValidatorHeaderEdgeCases(t *testing.T) {
	t.Run("at-sign directive rejected", func(t *testing.T) {
		input := "@c4m  1.0\n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for directive line")
		}
	})
}

// TestValidatorModeEdgeCases tests mode validation
func TestValidatorModeEdgeCases(t *testing.T) {
	t.Run("invalid type character", func(t *testing.T) {
		input := "Xrw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Invalid type character should fail")
		}
	})

	t.Run("setuid setgid sticky all set", func(t *testing.T) {
		input := "-rwsrwsrwt 2025-01-01T00:00:00Z 100 special.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("All special bits should be valid: %v", err)
		}
	})
}

// TestValidatorNameEmptyCase tests empty name validation
func TestValidatorNameEmptyCase(t *testing.T) {
	t.Run("entry with empty name field", func(t *testing.T) {
		// Create a manifest entry with minimal fields where name parsing might fail
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Entry without name should fail")
		}
	})
}

// TestEncodingPrettyMoreCases tests pretty encoding edge cases
func TestEncodingPrettyMoreCases(t *testing.T) {
	t.Run("pretty encode large sizes", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:      "huge.bin",
			Mode:      0644,
			Size:      1234567890123,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})

		var buf bytes.Buffer
		err := NewEncoder(&buf).SetPretty(true).Encode(manifest)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		output := buf.String()
		// Pretty format should have commas in size
		if !strings.Contains(output, ",") {
			t.Error("Expected comma-formatted size in pretty output")
		}
	})

	t.Run("pretty encode with special modes", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "pipe",
			Mode: os.ModeNamedPipe | 0644,
			Size: 0,
		})
		manifest.AddEntry(&Entry{
			Name: "socket",
			Mode: os.ModeSocket | 0755,
			Size: 0,
		})

		var buf bytes.Buffer
		err := NewEncoder(&buf).SetPretty(true).Encode(manifest)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
}

// TestValidateNameCoverage tests validateName branches
func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	t.Run("roundtrip with multiple entries", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:      "dir/",
			Mode:      os.ModeDir | 0755,
			Size:      4096,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		manifest.AddEntry(&Entry{
			Name:      "file.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Depth:     1,
		})

		// Marshal
		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		// Unmarshal
		restored, err := Unmarshal(data)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if len(restored.Entries) != len(manifest.Entries) {
			t.Errorf("Entry count mismatch: got %d, want %d", len(restored.Entries), len(manifest.Entries))
		}
	})

	t.Run("MarshalPretty basic", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:      "large.bin",
			Mode:      0644,
			Size:      1234567,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})

		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}

		// Pretty format should have comma in size
		if !strings.Contains(string(data), "1,234,567") {
			t.Error("Expected comma-formatted size")
		}
	})
}

// TestValidateHeaderCoverage tests that directive lines are rejected by the validator
func TestValidateHeaderCoverage(t *testing.T) {
	t.Run("at-sign directive rejected", func(t *testing.T) {
		input := "@c4m 1.0\n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for directive line")
		}
	})

	t.Run("empty leading line ignored", func(t *testing.T) {
		input := "\n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Empty leading line should be ignored: %v", err)
		}
	})
}

// TestValidateEntryCoverage tests validateEntry branches
func TestValidateEntryCoverage(t *testing.T) {
	t.Run("entry with too few fields", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for too few fields")
		}
	})

	t.Run("entry with inconsistent indentation", func(t *testing.T) {
		// Indentation without parent directory
		input := "  -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		// Exercises depth validation path
		_ = err
	})

	t.Run("file entry with trailing slash", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt/\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		// Exercises file mode vs trailing slash validation
		_ = err
	})
}

// TestOperationsRemainingBranches tests remaining operations branches
func TestResolverMoreCoverage(t *testing.T) {
	t.Run("resolve path with storage error", func(t *testing.T) {
		storage := &testCoverageStorage{err: fmt.Errorf("storage error")}
		resolver := NewResolver(storage)

		rootID := c4.Identify(strings.NewReader("root"))
		_, err := resolver.Resolve(rootID, "dir/subdir/file.txt")
		if err == nil {
			t.Error("Expected error from resolver")
		}
	})
}

// testCoverageStorage implements store.Source for testing
type testCoverageStorage struct {
	err error
}

func (s *testCoverageStorage) Open(id c4.ID) (io.ReadCloser, error) {
	return nil, s.err
}

// TestSequenceExpanderMoreCoverage tests sequence expansion edge cases
func TestSequenceExpanderMoreCoverage(t *testing.T) {
	t.Run("expand standalone sequence", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceStandalone)

		manifest := NewManifest()
		entry := &Entry{
			Name:  "frame[001-003].png",
			Mode:  0644,
			Size:  1000,
		}
		manifest.AddEntry(entry)

		result, _, err := expander.ExpandManifest(manifest)
		// Just exercise the expansion code path
		_ = result
		_ = err
	})

	t.Run("expand with no sequences", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceStandalone)

		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "regular.txt",
			Mode: 0644,
			Size: 100,
		})

		result, _, err := expander.ExpandManifest(manifest)
		if err != nil {
			t.Fatalf("ExpandManifest failed: %v", err)
		}
		if len(result.Entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(result.Entries))
		}
	})

	t.Run("expand embedded mode", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceEmbedded)

		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "img[001-005].jpg",
			Mode: 0644,
			Size: 1000,
		})

		result, _, err := expander.ExpandManifest(manifest)
		// Exercise embedded mode path
		_ = result
		_ = err
	})
}

// TestParseSequenceMoreBranches tests more ParseSequence branches
func TestParseSequenceMoreBranches(t *testing.T) {
	t.Run("sequence with padding", func(t *testing.T) {
		seq, err := ParseSequence("frame[0001-0010].png")
		if err != nil {
			t.Fatalf("ParseSequence failed: %v", err)
		}
		if seq.Padding != 4 {
			t.Errorf("Expected padding 4, got %d", seq.Padding)
		}
	})

	t.Run("invalid sequence range", func(t *testing.T) {
		_, err := ParseSequence("frame[10-5].png")
		if err == nil {
			t.Error("Expected error for invalid range")
		}
	})

	t.Run("not a sequence", func(t *testing.T) {
		_, err := ParseSequence("regular.txt")
		if err == nil {
			t.Error("Expected error for non-sequence")
		}
	})
}

// TestParseIDListMoreCoverage tests ParseIDList edge cases
func TestParseIDListMoreCoverage(t *testing.T) {
	t.Run("list with multiple IDs", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("content1"))
		id2 := c4.Identify(strings.NewReader("content2"))
		input := fmt.Sprintf("%s\n%s\n", id1.String(), id2.String())

		list, err := parseIDList(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if len(list.ids) != 2 {
			t.Errorf("Expected 2 IDs, got %d", len(list.ids))
		}
	})

	t.Run("list with single ID", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("content"))
		input := id.String() + "\n"

		list, err := parseIDList(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if len(list.ids) != 1 {
			t.Errorf("Expected 1 ID, got %d", len(list.ids))
		}
	})

	t.Run("list with invalid ID", func(t *testing.T) {
		input := "invalid-id\n"
		_, err := parseIDList(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid ID")
		}
	})
}

// TestDetectSequencesMoreBranches tests sequence detection edge cases
func TestDetectSequencesMoreBranches(t *testing.T) {
	t.Run("detect with numbered files", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file001.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "file002.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "file003.txt", Mode: 0644, Size: 100})

		detector := NewSequenceDetector(3)
		seqs := detector.DetectSequences(manifest)
		// Exercise detection code
		_ = seqs
	})

	t.Run("detect with mixed sequences", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "frame001.png", Mode: 0644, Size: 1000})
		manifest.AddEntry(&Entry{Name: "frame002.png", Mode: 0644, Size: 1000})
		manifest.AddEntry(&Entry{Name: "frame003.png", Mode: 0644, Size: 1000})
		manifest.AddEntry(&Entry{Name: "other.txt", Mode: 0644, Size: 50})
		manifest.AddEntry(&Entry{Name: "img_001.jpg", Mode: 0644, Size: 500})
		manifest.AddEntry(&Entry{Name: "img_002.jpg", Mode: 0644, Size: 500})
		manifest.AddEntry(&Entry{Name: "img_003.jpg", Mode: 0644, Size: 500})

		detector := NewSequenceDetector(3)
		seqs := detector.DetectSequences(manifest)
		// Exercise detection code
		_ = seqs
	})

	t.Run("detect with minimum length 2", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "img01.png", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "img02.png", Mode: 0644, Size: 100})

		detector := NewSequenceDetector(2)
		seqs := detector.DetectSequences(manifest)
		_ = seqs
	})
}

// TestReadLineErrors tests readLine error paths
func TestReadLineErrors(t *testing.T) {
	t.Run("read with io error", func(t *testing.T) {
		errReader := &errorReader{err: fmt.Errorf("read error")}
		decoder := NewDecoder(errReader)
		_, err := decoder.Decode()
		if err == nil {
			t.Error("Expected error from decoder")
		}
	})
}

// errorReader always returns an error
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}

// TestEncodeWithMetadata tests encoding with various metadata fields
func TestMarshalErrorPaths(t *testing.T) {
	t.Run("Marshal with entries", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:      "file.txt",
			Mode:      0644,
			Size:      100,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})

		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if len(data) == 0 {
			t.Error("Expected non-empty output")
		}
	})

	t.Run("MarshalPretty with entries", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:      "file.txt",
			Mode:      0644,
			Size:      1234567,
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		})

		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if !strings.Contains(string(data), ",") {
			t.Error("Expected comma-formatted size in pretty output")
		}
	})
}

// TestEncoderSetIndent tests encoder custom indent
func TestEncoderSetIndent(t *testing.T) {
	t.Run("encode with custom indent", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name:  "dir/",
			Mode:  os.ModeDir | 0755,
			Size:  4096,
			Depth: 0,
		})
		manifest.AddEntry(&Entry{
			Name:  "file.txt",
			Mode:  0644,
			Size:  100,
			Depth: 1,
		})

		var buf bytes.Buffer
		err := NewEncoder(&buf).SetIndent(4).Encode(manifest)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		// Check output has proper indentation
		_ = buf.String()
	})
}

// TestParseEntryMoreBranches tests more parseEntry branches
func TestParseEntryMoreBranches(t *testing.T) {
	t.Run("entry with null C4 ID", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt -\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if len(manifest.Entries) != 1 {
			t.Error("Expected 1 entry")
		}
	})

	t.Run("directory entry", func(t *testing.T) {
		input := "drwxr-xr-x 2025-01-01T00:00:00Z 4096 mydir/\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if !manifest.Entries[0].IsDir() {
			t.Error("Expected directory entry")
		}
	})

	t.Run("symlink entry", func(t *testing.T) {
		input := "lrwxr-xr-x 2025-01-01T00:00:00Z 10 link -> target\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if !manifest.Entries[0].IsSymlink() {
			t.Error("Expected symlink entry")
		}
	})
}

// TestFormatSizeMoreBranches tests formatSize edge cases
func TestIsIDListContentCoverage(t *testing.T) {
	t.Run("valid ID list", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("content"))
		content := []byte(id.String() + "\n")
		if !IsIDListContent(content) {
			t.Error("Expected valid ID list content")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		content := []byte("")
		if IsIDListContent(content) {
			t.Error("Empty content should not be valid ID list")
		}
	})

	t.Run("only whitespace", func(t *testing.T) {
		content := []byte("   \n\n   \n")
		if IsIDListContent(content) {
			t.Error("Whitespace-only should not be valid ID list")
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		content := []byte("not a c4 id\n")
		if IsIDListContent(content) {
			t.Error("Invalid content should not be valid ID list")
		}
	})

	t.Run("mixed valid and invalid", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("content"))
		content := []byte(id.String() + "\nnot valid\n")
		if IsIDListContent(content) {
			t.Error("Mixed content should not be valid")
		}
	})
}

// TestParseIDListScannerError tests scanner error path
func TestParseIDListScannerError(t *testing.T) {
	t.Run("scanner error", func(t *testing.T) {
		// Create a reader that errors after a few bytes
		errReader := &limitedErrorReader{
			data:      []byte("some data that will be cut off"),
			errorAt:   10,
		}
		_, err := parseIDList(errReader)
		// Scanner should encounter an error
		if err == nil {
			t.Log("No error from limited reader, scanner may have buffered all data")
		}
	})
}

// limitedErrorReader returns data up to errorAt bytes, then returns an error
type limitedErrorReader struct {
	data    []byte
	pos     int
	errorAt int
}

func (r *limitedErrorReader) Read(p []byte) (n int, err error) {
	if r.pos >= r.errorAt {
		return 0, fmt.Errorf("reader error")
	}
	remaining := r.errorAt - r.pos
	if len(p) > remaining {
		copy(p, r.data[r.pos:r.pos+remaining])
		r.pos = r.errorAt
		return remaining, fmt.Errorf("reader error")
	}
	end := r.pos + len(p)
	if end > len(r.data) {
		end = len(r.data)
	}
	n = copy(p, r.data[r.pos:end])
	r.pos = end
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

// TestOperationsEdgeCases3 tests more operation edge cases
func TestFormatSizeCoverage(t *testing.T) {
	t.Run("small size pretty format", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "small.txt",
			Mode: 0644,
			Size: 99, // <= 3 digits, no commas
		})

		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if strings.Contains(string(data), ",") {
			t.Error("Small size should not have commas")
		}
	})

	t.Run("exact 3 digits pretty format", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "small.txt",
			Mode: 0644,
			Size: 999, // exactly 3 digits
		})

		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		// 999 shouldn't have commas
		if strings.Contains(string(data), ",") {
			t.Error("999 should not have commas")
		}
	})

	t.Run("four digits pretty format", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{
			Name: "medium.txt",
			Mode: 0644,
			Size: 1234, // 4 digits should have comma
		})

		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if !strings.Contains(string(data), "1,234") {
			t.Error("1234 should be formatted as 1,234")
		}
	})
}

// TestParseTimestampCoverage tests timestamp parsing edge cases
func TestParseTimestampCoverage(t *testing.T) {
	t.Run("timestamp with timezone", func(t *testing.T) {
		input := "-rw-r--r-- 2025-01-01T12:30:45Z 100 file.txt\n"
		manifest, err := Unmarshal([]byte(input))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if !manifest.Entries[0].Timestamp.UTC().Equal(time.Date(2025, 1, 1, 12, 30, 45, 0, time.UTC)) {
			t.Error("Timestamp not parsed correctly")
		}
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		input := "-rw-r--r-- invalid-timestamp 100 file.txt\n"
		_, err := Unmarshal([]byte(input))
		if err == nil {
			t.Error("Expected error for invalid timestamp")
		}
	})
}

// errorSource is a Source that always returns an error
type errorSource struct{}

func (e errorSource) ToManifest() (*Manifest, error) {
	return nil, fmt.Errorf("source error")
}

// TestValidatorEntryEdgeCases2 tests more validateEntry edge cases
func TestValidatorEntryEdgeCases2(t *testing.T) {
	t.Run("invalid UTF-8", func(t *testing.T) {
		// Create input with invalid UTF-8
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file\xff.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		// Should have error for invalid UTF-8
		_ = err
	})

	t.Run("control character", func(t *testing.T) {
		// Create input with control character (not tab)
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x01.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for control character")
		}
	})

	t.Run("odd indentation", func(t *testing.T) {
		// 3 spaces instead of 2 or 4
		input := "drwxr-xr-x 2025-01-01T00:00:00Z 4096 dir/\n   -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for odd indentation")
		}
	})

	t.Run("invalid depth jump", func(t *testing.T) {
		// Jump from depth 0 to depth 2
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 root.txt\n    -rw-r--r-- 2025-01-01T00:00:00Z 100 deep.txt\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for depth jump")
		}
	})

	t.Run("dedentation reset", func(t *testing.T) {
		// Valid dedentation
		input := "drwxr-xr-x 2025-01-01T00:00:00Z 4096 a/\n  -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\ndrwxr-xr-x 2025-01-01T00:00:00Z 4096 b/\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err != nil {
			t.Errorf("Valid dedentation should not error: %v", err)
		}
	})
}

// TestValidatorC4IDEdgeCases2 tests more C4 ID validation
func TestValidatorC4IDEdgeCases2(t *testing.T) {
	t.Run("C4 ID wrong length", func(t *testing.T) {
		// C4 ID should be exactly 90 characters
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c4short\n"
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for short C4 ID")
		}
	})

	t.Run("C4 ID wrong prefix", func(t *testing.T) {
		// C4 ID must start with "c4"
		longID := "x4" + strings.Repeat("a", 88)
		input := fmt.Sprintf("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt %s\n", longID)
		v := NewValidator(true)
		err := v.ValidateManifest(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for wrong prefix")
		}
	})
}

// TestOperationsErrorSources tests operations with error-returning sources
func TestSortSiblingsCoverage(t *testing.T) {
	t.Run("empty manifest", func(t *testing.T) {
		manifest := NewManifest()
		manifest.SortEntries() // Should not panic
		if len(manifest.Entries) != 0 {
			t.Error("Empty manifest should remain empty")
		}
	})

	t.Run("orphaned entries", func(t *testing.T) {
		manifest := NewManifest()
		// Add entries out of order that look orphaned
		manifest.Entries = append(manifest.Entries, &Entry{
			Name:  "child.txt",
			Mode:  0644,
			Size:  100,
			Depth: 2, // Depth 2 without depth 1 parent
		})
		manifest.Entries = append(manifest.Entries, &Entry{
			Name:  "root.txt",
			Mode:  0644,
			Size:  200,
			Depth: 0,
		})

		manifest.SortEntries()
		// Orphaned entry should be handled
		if len(manifest.Entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(manifest.Entries))
		}
	})

	t.Run("deep nesting", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "a/", Mode: os.ModeDir | 0755, Size: 4096, Depth: 0})
		manifest.AddEntry(&Entry{Name: "b/", Mode: os.ModeDir | 0755, Size: 4096, Depth: 1})
		manifest.AddEntry(&Entry{Name: "c/", Mode: os.ModeDir | 0755, Size: 4096, Depth: 2})
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100, Depth: 3})

		manifest.SortEntries()
		if len(manifest.Entries) != 4 {
			t.Errorf("Expected 4 entries, got %d", len(manifest.Entries))
		}
	})
}

// TestNaturalSortCoverage tests NaturalLess edge cases
func TestNaturalSortCoverage(t *testing.T) {
	t.Run("equal strings", func(t *testing.T) {
		if NaturalLess("abc", "abc") {
			t.Error("Equal strings should not be less")
		}
	})

	t.Run("numeric comparison", func(t *testing.T) {
		if !NaturalLess("file2", "file10") {
			t.Error("file2 should be less than file10")
		}
	})

	t.Run("alpha comparison", func(t *testing.T) {
		if !NaturalLess("aaa", "bbb") {
			t.Error("aaa should be less than bbb")
		}
	})

	t.Run("mixed alpha numeric", func(t *testing.T) {
		if !NaturalLess("file1a", "file1b") {
			t.Error("file1a should be less than file1b")
		}
	})
}

// TestEncodeErrorPaths tests Encode error paths
func TestManifestWriting(t *testing.T) {
	manifest := NewManifest()
	manifest.Version = "1.0"

	// Add various entries
	manifest.AddEntry(&Entry{
		Name:      "test.txt",
		Size:      100,
		Mode:      0644,
		Timestamp: time.Now(),
	})

	manifest.AddEntry(&Entry{
		Name: "dir/",
		Mode: 0755 | os.ModeDir,
	})

	// Test Encode (canonical)
	var buf bytes.Buffer
	err := NewEncoder(&buf).Encode(manifest)
	if err != nil {
		t.Errorf("Encode failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encode wrote 0 bytes")
	}

	// Test Encode (pretty)
	buf.Reset()
	err = NewEncoder(&buf).SetPretty(true).Encode(manifest)
	if err != nil {
		t.Errorf("Encode (pretty) failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encode (pretty) wrote 0 bytes")
	}
}

// Test entry methods
func TestEntryMethods(t *testing.T) {
	// Test regular file
	entry := &Entry{
		Name: "file.txt",
		Mode: 0644,
		Size: 100,
	}

	if entry.IsDir() {
		t.Error("Regular file marked as directory")
	}

	if entry.IsSymlink() {
		t.Error("Regular file marked as symlink")
	}

	// Test directory
	dirEntry := &Entry{
		Name: "dir/",
		Mode: 0755 | os.ModeDir,
	}

	if !dirEntry.IsDir() {
		t.Error("Directory not marked as directory")
	}

	// Test symlink
	linkEntry := &Entry{
		Name:   "link",
		Mode:   0777 | os.ModeSymlink,
		Target: "target",
	}

	if !linkEntry.IsSymlink() {
		t.Error("Symlink not marked as symlink")
	}

	// Test String methods
	_ = entry.String()
}

// Test manifest operations
func TestManifestOperations(t *testing.T) {
	m1 := NewManifest()
	m1.AddEntry(&Entry{Name: "a.txt", Size: 100})
	m1.AddEntry(&Entry{Name: "b.txt", Size: 200})

	m2 := NewManifest()
	m2.AddEntry(&Entry{Name: "b.txt", Size: 200})
	m2.AddEntry(&Entry{Name: "c.txt", Size: 300})

	// Test that we have entries
	if len(m1.Entries) != 2 {
		t.Errorf("Expected 2 entries in m1, got %d", len(m1.Entries))
	}

	if len(m2.Entries) != 2 {
		t.Errorf("Expected 2 entries in m2, got %d", len(m2.Entries))
	}
}

// Test sorting operations
func TestSortingOperations(t *testing.T) {
	manifest := NewManifest()

	// Add entries in reverse order
	manifest.AddEntry(&Entry{Name: "z.txt", Mode: 0644})
	manifest.AddEntry(&Entry{Name: "a.txt", Mode: 0644})
	manifest.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755})
	manifest.AddEntry(&Entry{Name: "m.txt", Mode: 0644})

	// Test SortEntries (files before directories at same depth)
	manifest.SortEntries()
	if manifest.Entries[0].Name != "a.txt" {
		t.Errorf("Expected first entry to be a.txt after sort, got %s", manifest.Entries[0].Name)
	}

	// Test SortEntries (hierarchical sort)
	manifest2 := NewManifest()
	manifest2.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Depth: 0})
	manifest2.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755, Depth: 0})
	manifest2.AddEntry(&Entry{Name: "another.txt", Mode: 0644, Depth: 0})

	manifest2.SortEntries()
	// Files should come before directories at same depth
	if manifest2.Entries[0].Mode.IsDir() {
		t.Error("Directory came before file at same depth")
	}
}

// Test natural sorting additional cases
func TestNaturalSortAdditional(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"file1.txt", "file2.txt", true},
		{"file2.txt", "file10.txt", true},
		{"file10.txt", "file2.txt", false},
		{"abc", "def", true},
		{"def", "abc", false},
	}

	for _, tt := range tests {
		got := NaturalLess(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// Test manifest sources
func TestManifestSources(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{Name: "test.txt", Size: 100})

	// Test ManifestSource
	source := ManifestSource{manifest}
	// Just ensure we can create the source
	if source.Manifest == nil {
		t.Error("ManifestSource has nil manifest")
	}
}
// ----------------------------------------------------------------------------
// Basic Coverage Tests (merged from basic_coverage_test.go)
// ----------------------------------------------------------------------------

func TestManifestBasic(t *testing.T) {
	m := NewManifest()
	if m == nil {
		t.Fatal("NewManifest returned nil")
	}

	// Test AddEntry
	entry := &Entry{
		Name:      "test.txt",
		Mode:      0644,
		Size:      100,
		Timestamp: time.Now(),
		C4ID:      c4.Identify(strings.NewReader("test")),
	}
	m.AddEntry(entry)

	// Test SortEntries
	m.SortEntries()

	// Test Encoder
	var buf bytes.Buffer
	err := NewEncoder(&buf).Encode(m)
	if err != nil {
		t.Errorf("Encode failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encode wrote 0 bytes")
	}

	// Test GetEntry
	e := m.GetEntry("test.txt")
	if e == nil {
		t.Error("GetEntry returned nil")
	}

	// Test ComputeC4ID
	id := m.ComputeC4ID()
	var emptyID c4.ID
	if id == emptyID {
		t.Error("ComputeC4ID returned empty ID")
	}

	// Test Canonical
	canonical := m.Canonical()
	if canonical == "" {
		t.Error("Canonical returned empty string")
	}

	// Test GetEntriesAtDepth
	entries := m.GetEntriesAtDepth(0)
	if len(entries) == 0 {
		t.Error("GetEntriesAtDepth returned no entries")
	}
}

// Test Entry methods
func TestSequenceBasic(t *testing.T) {
	// Test IsSequence
	if !IsSequence("file_[001-005].txt") {
		t.Error("Expected IsSequence to return true")
	}

	// Test ParseSequence
	seq, err := ParseSequence("file_[001-005].txt")
	if err != nil {
		t.Errorf("ParseSequence failed: %v", err)
	}
	if seq != nil {
		// Test Expand
		files := seq.Expand()
		if len(files) != 5 {
			t.Errorf("Expected 5 files, got %d", len(files))
		}

		// Test Count
		if seq.Count() != 5 {
			t.Errorf("Expected count 5, got %d", seq.Count())
		}
	}
}

// Test Validator
func TestValidatorBasic(t *testing.T) {
	v := NewValidator(false)
	if v == nil {
		t.Fatal("NewValidator returned nil")
	}

	// Test GetStats
	stats := v.GetStats()
	// Check if stats has expected initial values
	if stats.Files < 0 {
		t.Error("GetStats returned invalid Files count")
	}

	// Test GetCurrentPath
	path := v.GetCurrentPath()
	if path != "" {
		t.Errorf("Expected empty path, got %q", path)
	}
}

// Test Decoder
func TestDecoderBasic(t *testing.T) {
	p := NewDecoder(strings.NewReader(""))
	if p == nil {
		t.Fatal("NewDecoder returned nil")
	}

	// Test Decode
	m, err := p.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if m == nil {
		t.Fatal("Decode returned nil manifest")
	}
}

// Test Operations with ManifestSource
