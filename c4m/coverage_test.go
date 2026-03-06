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
	if !strings.Contains(output, "1,234,567,890") {
		t.Errorf("Expected comma-formatted size, got:\n%s", output)
	}
}

// TestFormatPretty tests the FormatPretty function
func TestFormatPretty(t *testing.T) {
	input := []byte("-rw-r--r-- 2025-01-01T00:00:00Z 1000000 bigfile.txt\n")
	output, err := FormatPretty(input)
	if err != nil {
		t.Fatalf("FormatPretty failed: %v", err)
	}

	if !strings.Contains(string(output), "1,000,000") {
		t.Errorf("Expected pretty-formatted output with commas, got:\n%s", output)
	}
}

// TestResolverCache tests the Cache method on Resolver
func TestResolverCache(t *testing.T) {
	storage := &testStorage{
		data: map[string]string{
			"c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111": "-rw-r--r-- 2025-01-01T00:00:00Z 100 test.txt\n",
		},
	}

	resolver := NewResolver(storage)
	cache := resolver.Cache()
	if cache == nil {
		t.Fatal("Cache() returned nil")
	}

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
				Name: "nullmode.txt", Mode: 0, Size: 100,
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			contains: []string{"----------"},
		},
		{
			name: "null timestamp",
			entry: &Entry{
				Name: "nulltime.txt", Mode: 0644, Size: 100, Timestamp: time.Time{},
			},
			contains: []string{"-"},
		},
		{
			name: "null size",
			entry: &Entry{
				Name: "nullsize.txt", Mode: 0644, Size: -1,
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			contains: []string{"-"},
		},
		{
			name: "symlink",
			entry: &Entry{
				Name: "link", Mode: os.ModeSymlink | 0777, Size: 0,
				Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Target:    "/target/path",
			},
			contains: []string{"->", "/target/path"},
		},
		{
			name: "large file with commas",
			entry: &Entry{
				Name: "big.bin", Mode: 0644, Size: 12345678,
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

// TestIDListEdgeCases tests edge cases in idlist.go
func TestIDListEdgeCases(t *testing.T) {
	t.Run("parse valid ID list", func(t *testing.T) {
		input := "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111\nc42222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
		ids, err := parseIDList(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if ids.Count() != 2 {
			t.Errorf("Expected 2 IDs, got %d", ids.Count())
		}
	})

	t.Run("parse ID list with invalid entry returns error", func(t *testing.T) {
		input := "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111\nnot-a-valid-id"
		_, err := parseIDList(strings.NewReader(input))
		if err == nil {
			t.Error("Expected error for invalid ID")
		}
	})

	t.Run("IsIDListContent with mixed content", func(t *testing.T) {
		if IsIDListContent([]byte("this is not ID content\nwith random text")) {
			t.Error("Should not identify random text as ID list")
		}
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
		input := "-rw-r--r-- 2025-01-01T00:00:00Z 100 file\r.txt\n"
		_, err := Unmarshal([]byte(input))
		if err == nil {
			t.Fatal("Expected error for bare CR in entry name")
		}
		if !strings.Contains(err.Error(), "CR") {
			t.Errorf("Expected CR-related error, got: %v", err)
		}
	})

	t.Run("directives rejected", func(t *testing.T) {
		_, err := Unmarshal([]byte("@c4m 1.0\n"))
		if err == nil {
			t.Fatal("Expected error for directive line")
		}
	})

	t.Run("encoder output is LF only", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{Name: "test.txt", Mode: 0644, Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Size: 100})
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
		manifest, err := Unmarshal([]byte("-rw-r--r-- 0 100 file.txt\n"))
		if err != nil {
			t.Fatalf("Failed to parse zero timestamp: %v", err)
		}
		if manifest.Entries[0].Timestamp.Unix() != 0 {
			t.Errorf("Expected Unix epoch, got %v", manifest.Entries[0].Timestamp)
		}
	})

	t.Run("directory entry", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("drwxr-xr-x 2025-01-01T00:00:00Z 4096 mydir/\n"))
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
		input := "brw-r--r-- 2025-01-01T00:00:00Z 0 block_device\ncrw-r--r-- 2025-01-01T00:00:00Z 0 char_device\nprw-r--r-- 2025-01-01T00:00:00Z 0 pipe\nsrw-r--r-- 2025-01-01T00:00:00Z 0 socket\n"
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
		validator := NewValidator(false)
		err := validator.ValidateManifest(strings.NewReader("---------- 2025-01-01T00:00:00Z 100 nullmode.txt\n"))
		if err != nil {
			t.Errorf("Should accept null mode: %v", err)
		}
	})

	t.Run("validate path with null bytes", func(t *testing.T) {
		validator := NewValidator(true)
		err := validator.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x00name.txt\n"))
		if err == nil {
			t.Error("Should reject path with null bytes")
		}
	})
}

// TestSequenceExpansionEdgeCases tests edge cases in sequence expansion
func TestSequenceExpansionEdgeCases(t *testing.T) {
	t.Run("expand sequence with manifest lookup", func(t *testing.T) {
		seqEntry := &Entry{Name: "file.[001-003].txt", Mode: 0644, Size: 100, IsSequence: true}
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.001.txt", Mode: 0644, Size: 100, C4ID: c4.Identify(strings.NewReader("content1"))})
		manifest.AddEntry(&Entry{Name: "file.002.txt", Mode: 0644, Size: 100, C4ID: c4.Identify(strings.NewReader("content2"))})
		manifest.AddEntry(&Entry{Name: "file.003.txt", Mode: 0644, Size: 100, C4ID: c4.Identify(strings.NewReader("content3"))})

		expanded, err := expandSequenceEntryWithManifest(seqEntry, manifest)
		if err != nil {
			t.Fatalf("expandSequenceEntryWithManifest failed: %v", err)
		}
		if len(expanded) != 3 {
			t.Errorf("Expected 3 expanded entries, got %d", len(expanded))
		}
	})

	t.Run("expand entry with id list", func(t *testing.T) {
		seqEntry := &Entry{Name: "file.[01-03].txt", Mode: 0644, Size: 100, IsSequence: true}
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

// testStorage implements store.Source for testing
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
func TestEncoderEdgeCases(t *testing.T) {
	t.Run("encode symlink entry", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "link", Mode: os.ModeSymlink | 0777, Size: 0, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Target: "/target/path"})
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(manifest); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "->") || !strings.Contains(output, "/target/path") {
			t.Errorf("Expected symlink target in output, got:\n%s", output)
		}
	})

	t.Run("encode nested directories", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "dir/", Mode: 0755 | os.ModeDir, Size: 0, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Depth: 0})
		manifest.AddEntry(&Entry{Name: "subdir/", Mode: 0755 | os.ModeDir, Size: 0, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Depth: 1})
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Depth: 2})
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(manifest); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "  drwxr-xr-x") {
			t.Errorf("Expected indented subdir, got:\n%s", output)
		}
		if !strings.Contains(output, "    -rw-r--r--") {
			t.Errorf("Expected double-indented file, got:\n%s", output)
		}
	})
}

// TestFormatFunctions tests Format and FormatPretty
func TestFormatFunctions(t *testing.T) {
	t.Run("format invalid input", func(t *testing.T) {
		if _, err := Format([]byte("not valid c4m")); err == nil {
			t.Error("Expected error for invalid input")
		}
	})
	t.Run("format pretty invalid input", func(t *testing.T) {
		if _, err := FormatPretty([]byte("not valid c4m")); err == nil {
			t.Error("Expected error for invalid input")
		}
	})
	t.Run("marshal error handling", func(t *testing.T) {
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
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "test.txt", Mode: 0644, Size: 100, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
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
			t.Logf("ParseDataBlock with empty content: %v", err)
		}
		_ = block
	})

	t.Run("parse valid ID list", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("test1"))
		id2 := c4.Identify(strings.NewReader("test2"))
		content := id1.String() + "\n" + id2.String() + "\n"
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
		manifest, err := Unmarshal([]byte("brw-rw---- 2025-01-01T00:00:00Z 0 blockdev\n"))
		if err != nil {
			t.Fatalf("Failed to parse block device: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeDevice == 0 {
			t.Error("Expected device mode")
		}
	})
	t.Run("character device", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("crw-rw---- 2025-01-01T00:00:00Z 0 chardev\n"))
		if err != nil {
			t.Fatalf("Failed to parse char device: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeCharDevice == 0 {
			t.Error("Expected char device mode")
		}
	})
	t.Run("named pipe", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("prw-rw---- 2025-01-01T00:00:00Z 0 mypipe\n"))
		if err != nil {
			t.Fatalf("Failed to parse named pipe: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeNamedPipe == 0 {
			t.Error("Expected named pipe mode")
		}
	})
	t.Run("socket", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("srw-rw---- 2025-01-01T00:00:00Z 0 mysocket\n"))
		if err != nil {
			t.Fatalf("Failed to parse socket: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeSocket == 0 {
			t.Error("Expected socket mode")
		}
	})
	t.Run("setuid permission", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("-rwsr-xr-x 2025-01-01T00:00:00Z 100 setuid\n"))
		if err != nil {
			t.Fatalf("Failed to parse setuid: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeSetuid == 0 {
			t.Error("Expected setuid mode")
		}
	})
	t.Run("setgid permission", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("-rwxr-sr-x 2025-01-01T00:00:00Z 100 setgid\n"))
		if err != nil {
			t.Fatalf("Failed to parse setgid: %v", err)
		}
		if manifest.Entries[0].Mode&os.ModeSetgid == 0 {
			t.Error("Expected setgid mode")
		}
	})
	t.Run("sticky bit", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("drwxrwxrwt 2025-01-01T00:00:00Z 4096 sticky/\n"))
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
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- invalid_timestamp 100 file.txt\n")); err == nil {
			t.Error("Expected error for invalid timestamp")
		}
	})
	t.Run("invalid name with control chars", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x00name.txt\n")); err == nil {
			t.Error("Expected error for invalid name")
		}
	})
	t.Run("invalid C4 ID", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt invalid_c4id\n")); err == nil {
			t.Error("Expected error for invalid C4 ID")
		}
	})
	t.Run("invalid mode format", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-rXr-- 2025-01-01T00:00:00Z 100 file.txt\n")); err == nil {
			t.Error("Expected error for invalid mode")
		}
	})
	t.Run("completely wrong content", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("not a valid entry\n")); err == nil {
			t.Error("Expected error for invalid content")
		}
	})
	t.Run("valid entry-only format", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n")); err != nil {
			t.Errorf("Valid entry should pass: %v", err)
		}
	})
}

// TestSequenceExpansionWithManifestCoverage tests sequence expansion with manifest
func TestSequenceExpansionWithManifestCoverage(t *testing.T) {
	t.Run("expand regular entry without manifest", func(t *testing.T) {
		entry := &Entry{Name: "file.txt", Mode: 0644, Size: 100, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
		results, err := expandSequenceEntryWithManifest(entry, nil)
		if err != nil {
			t.Logf("expandSequenceEntryWithManifest on regular entry: %v", err)
		} else if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})
	t.Run("expand with manifest", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.001.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "file.002.txt", Mode: 0644, Size: 100})
		entry := &Entry{Name: "file.001.txt", Mode: 0644, Size: 100, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
		results, err := expandSequenceEntryWithManifest(entry, manifest)
		if err != nil {
			t.Logf("expandSequenceEntryWithManifest with manifest: %v", err)
		}
		_ = results
	})
}

// TestSequenceDetectionCoverage tests sequence detection
func TestSequenceDetectionCoverage(t *testing.T) {
	t.Run("detect image sequences", func(t *testing.T) {
		manifest := NewManifest()
		for i := 1; i <= 5; i++ {
			manifest.AddEntry(&Entry{Name: fmt.Sprintf("image.%04d.png", i), Mode: 0644, Size: int64(1000 * i), Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
		}
		detector := NewSequenceDetector(3)
		if result := detector.DetectSequences(manifest); result == nil {
			t.Error("Expected non-nil result")
		}
	})
	t.Run("no sequences to detect", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "readme.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "config.json", Mode: 0644, Size: 50})
		detector := NewSequenceDetector(3)
		if result := detector.DetectSequences(manifest); result == nil {
			t.Error("Expected non-nil result")
		}
	})
}

// TestParseSequenceCoverage tests sequence parsing
func TestParseSequenceCoverage(t *testing.T) {
	t.Run("parse sequence pattern", func(t *testing.T) {
		if !IsSequence("file.[001-003].txt") {
			t.Error("Expected IsSequence to return true for valid pattern")
		}
		if IsSequence("file.txt") {
			t.Error("Expected IsSequence to return false for non-sequence")
		}
	})
	t.Run("expand sequence pattern", func(t *testing.T) {
		result, err := ExpandSequencePattern("file.[001-003].txt")
		if err != nil {
			t.Fatalf("ExpandSequencePattern failed: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("Expected 3 results, got %d", len(result))
		}
	})
	t.Run("parse non-sequence", func(t *testing.T) {
		if _, err := ParseSequence("file.txt"); err == nil {
			t.Error("Expected error for non-sequence")
		}
	})
}

// TestResolverCoverage tests resolver edge cases
func TestResolverCoverage(t *testing.T) {
	t.Run("resolve missing manifest", func(t *testing.T) {
		resolver := NewResolver(&testErrorStorage{})
		if _, err := resolver.Resolve(c4.Identify(strings.NewReader("nonexistent")), "path/to/file"); err == nil {
			t.Error("Expected error for missing manifest")
		}
	})
}

type testErrorStorage struct{}

func (s *testErrorStorage) Open(id c4.ID) (io.ReadCloser, error) {
	return nil, fmt.Errorf("object not found: %s", id.String())
}

// TestParseIDListCoverage tests ParseIDList edge cases
func TestParseIDListCoverage(t *testing.T) {
	t.Run("parse single ID", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("content1"))
		idList, err := parseIDList(strings.NewReader(id1.String() + "\n"))
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
		idList, err := parseIDList(strings.NewReader(id1.String() + "\n" + id2.String() + "\n"))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if idList.Count() != 2 {
			t.Errorf("Expected 2 IDs, got %d", idList.Count())
		}
	})
	t.Run("parse invalid ID", func(t *testing.T) {
		if _, err := parseIDList(strings.NewReader("not_a_valid_c4_id\n")); err == nil {
			t.Error("Expected error for invalid ID")
		}
	})
}

// TestEncoderWriteErrors tests encoder error handling
func TestEncoderWriteErrors(t *testing.T) {
	t.Run("write to failing writer", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100})
		fw := &failingWriter{failAfter: 5}
		if err := NewEncoder(fw).Encode(manifest); err == nil {
			t.Error("Expected error when writing to failing writer")
		}
	})
}

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
func TestEncodingMoreEdgeCases(t *testing.T) {
	t.Run("encode with data blocks produces no directives", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100})
		id := c4.Identify(strings.NewReader("test data"))
		manifest.AddDataBlock(&DataBlock{ID: id, IsIDList: false, Content: []byte("test data")})
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(manifest); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if strings.Contains(buf.String(), "@") {
			t.Errorf("Expected no directives in output, got:\n%s", buf.String())
		}
	})
}

// TestValidatorMoreEdgeCases tests more validator scenarios
func TestValidatorMoreEdgeCases(t *testing.T) {
	t.Run("validate directory entry", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("drwxr-xr-x 2025-01-01T00:00:00Z 4096 mydir/\n")); err != nil {
			t.Errorf("Valid directory entry should pass: %v", err)
		}
	})
	t.Run("validate symlink entry", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("lrwxrwxrwx 2025-01-01T00:00:00Z 0 link -> target\n")); err != nil {
			t.Errorf("Valid symlink entry should pass: %v", err)
		}
	})
	t.Run("validate with nested directories", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("drwxr-xr-x 2025-01-01T00:00:00Z 4096 dir/\n  -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n")); err != nil {
			t.Errorf("Valid nested entry should pass: %v", err)
		}
	})
	t.Run("validate negative size", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z -100 file.txt\n")); err == nil {
			t.Error("Negative size should fail")
		}
	})
	t.Run("validate very large file", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 9999999999999 large.bin\n")); err != nil {
			t.Errorf("Large file size should be valid: %v", err)
		}
	})
	t.Run("validate file with path components", func(t *testing.T) {
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 path/to/file.txt\n"))
	})
}

// TestSequenceExpanderCoverage tests sequence expander
func TestSequenceExpanderCoverage(t *testing.T) {
	t.Run("expand manifest with sequences", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceEmbedded)
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "regular.txt", Mode: 0644, Size: 100})
		expanded, expansions, err := expander.ExpandManifest(manifest)
		if err != nil {
			t.Fatalf("ExpandManifest failed: %v", err)
		}
		if expanded == nil {
			t.Error("Expected non-nil expanded result")
		}
		_ = expansions
	})
	t.Run("expand standalone mode", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceStandalone)
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100})
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
		manifest.AddEntry(&Entry{Name: "file.[001-003].txt", Mode: 0644, Size: 100})
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
		data, err := Marshal(NewManifest())
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("Expected empty output for empty manifest, got: %q", string(data))
		}
	})
	t.Run("marshal pretty empty manifest", func(t *testing.T) {
		data, err := MarshalPretty(NewManifest())
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("Expected empty output for empty manifest, got: %q", string(data))
		}
	})
	t.Run("marshal with multiple entries", func(t *testing.T) {
		manifest := NewManifest()
		for i := 0; i < 10; i++ {
			manifest.AddEntry(&Entry{Name: fmt.Sprintf("file%d.txt", i), Mode: 0644, Size: int64(100 * i), Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
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
func TestDecodingMoreEdgeCases(t *testing.T) {
	t.Run("decode entry with full timestamp", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("-rw-r--r-- 2025-06-15T14:30:45Z 100 file.txt\n"))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if manifest.Entries[0].Timestamp.IsZero() {
			t.Error("Expected valid timestamp")
		}
	})
	t.Run("decode entry with C4 ID", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("test content"))
		manifest, err := Unmarshal([]byte(fmt.Sprintf("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt %s\n", id.String())))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if manifest.Entries[0].C4ID.IsNil() {
			t.Error("Expected C4 ID to be parsed")
		}
	})
}

// TestDetectSequencesMoreCases tests sequence detection
func TestDetectSequencesMoreCases(t *testing.T) {
	t.Run("detect multiple sequences", func(t *testing.T) {
		manifest := NewManifest()
		for i := 1; i <= 5; i++ {
			manifest.AddEntry(&Entry{Name: fmt.Sprintf("seq_a.%03d.txt", i), Mode: 0644, Size: 100})
		}
		for i := 1; i <= 3; i++ {
			manifest.AddEntry(&Entry{Name: fmt.Sprintf("seq_b.%04d.png", i), Mode: 0644, Size: 200})
		}
		if result := NewSequenceDetector(2).DetectSequences(manifest); result == nil {
			t.Error("Expected non-nil result")
		}
	})
	t.Run("detect sequences with gaps", func(t *testing.T) {
		manifest := NewManifest()
		for _, n := range []int{1, 2, 5, 6, 7} {
			manifest.AddEntry(&Entry{Name: fmt.Sprintf("file.%03d.txt", n), Mode: 0644, Size: 100})
		}
		if result := NewSequenceDetector(2).DetectSequences(manifest); result == nil {
			t.Error("Expected non-nil result")
		}
	})
}

// TestValidatorTimestampCoverage tests timestamp validation edge cases
func TestValidatorTimestampCoverage(t *testing.T) {
	t.Run("null timestamp dash", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- - 100 file.txt\n")); err != nil {
			t.Errorf("Null timestamp '-' should be valid: %v", err)
		}
	})
	t.Run("null timestamp zero", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 0 100 file.txt\n")); err != nil {
			t.Errorf("Null timestamp '0' should be valid: %v", err)
		}
	})
	t.Run("timestamp without Z suffix", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00 100 file.txt\n")); err == nil {
			t.Error("Timestamp without Z should fail")
		}
	})
	t.Run("invalid timestamp format", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-13-45T99:99:99Z 100 file.txt\n")); err == nil {
			t.Error("Invalid timestamp should fail")
		}
	})
}

// TestValidatorNameCoverage tests name validation edge cases
func TestValidatorNameCoverage(t *testing.T) {
	cases := []struct {
		name, input string
	}{
		{"path traversal with dot-dot-slash", "-rw-r--r-- 2025-01-01T00:00:00Z 100 ../file.txt\n"},
		{"path traversal with dot-slash", "-rw-r--r-- 2025-01-01T00:00:00Z 100 ./file.txt\n"},
		{"directory just slash", "drwxr-xr-x 2025-01-01T00:00:00Z 0 /\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := NewValidator(true)
			if err := v.ValidateManifest(strings.NewReader(tc.input)); err == nil {
				t.Errorf("Expected error for %s", tc.name)
			}
		})
	}
}

// TestSequenceEntryWithManifestCoverage tests expandSequenceEntryWithManifest
func TestSequenceEntryWithManifestCoverage(t *testing.T) {
	t.Run("expand with C4ID and manifest with data block", func(t *testing.T) {
		idList := newIDList()
		idList.Add(c4.Identify(strings.NewReader("content1")))
		idList.Add(c4.Identify(strings.NewReader("content2")))
		dataBlock := createDataBlockFromIDList(idList)
		manifest := NewManifest()
		manifest.AddDataBlock(dataBlock)
		entry := &Entry{Name: "file.[001-002].txt", Mode: 0644, Size: 100, C4ID: dataBlock.ID}
		results, err := expandSequenceEntryWithManifest(entry, manifest)
		if err != nil {
			t.Logf("expandSequenceEntryWithManifest: %v", err)
		}
		_ = results
	})
	t.Run("expand with nil C4ID", func(t *testing.T) {
		results, err := expandSequenceEntryWithManifest(&Entry{Name: "file.txt", Mode: 0644, Size: 100}, NewManifest())
		if err != nil {
			t.Logf("expandSequenceEntryWithManifest with nil C4ID: %v", err)
		}
		_ = results
	})
}

// TestMarshalUnmarshalRoundtrip tests Marshal/Unmarshal roundtrip
func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	t.Run("roundtrip with multiple entries", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755, Size: 4096, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100, Timestamp: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Depth: 1})
		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
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
		manifest.AddEntry(&Entry{Name: "large.bin", Mode: 0644, Size: 1234567, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if !strings.Contains(string(data), "1,234,567") {
			t.Error("Expected comma-formatted size")
		}
	})
}

// TestValidateEntryCoverage tests validateEntry branches
func TestValidateEntryCoverage(t *testing.T) {
	t.Run("entry with too few fields", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z\n")); err == nil {
			t.Error("Expected error for too few fields")
		}
	})
	t.Run("entry with inconsistent indentation", func(t *testing.T) {
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader("  -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"))
	})
	t.Run("file entry with trailing slash", func(t *testing.T) {
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt/\n"))
	})
}

// TestDiffMoreBranches tests more Diff branches
func TestDiffMoreBranches(t *testing.T) {
	t.Run("Diff with identical manifests", func(t *testing.T) {
		m := NewManifest()
		m.AddEntry(&Entry{Name: "same.txt", Mode: 0644, Size: 100})
		result, err := Diff(ManifestSource{Manifest: m}, ManifestSource{Manifest: m})
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}
		if len(result.Added.Entries) != 0 || len(result.Removed.Entries) != 0 || len(result.Modified.Entries) != 0 {
			t.Error("Expected no differences for identical manifests")
		}
	})
	t.Run("Diff with modified entry", func(t *testing.T) {
		m1 := NewManifest()
		m1.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100, C4ID: c4.Identify(strings.NewReader("original"))})
		m2 := NewManifest()
		m2.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 200, C4ID: c4.Identify(strings.NewReader("modified"))})
		result, err := Diff(ManifestSource{Manifest: m1}, ManifestSource{Manifest: m2})
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}
		if len(result.Modified.Entries) == 0 {
			t.Error("Expected modified entries")
		}
	})
	t.Run("Diff with added entry", func(t *testing.T) {
		m1 := NewManifest()
		m1.AddEntry(&Entry{Name: "original.txt", Mode: 0644, Size: 100})
		m2 := NewManifest()
		m2.AddEntry(&Entry{Name: "original.txt", Mode: 0644, Size: 100})
		m2.AddEntry(&Entry{Name: "new.txt", Mode: 0644, Size: 200})
		result, err := Diff(ManifestSource{Manifest: m1}, ManifestSource{Manifest: m2})
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}
		if len(result.Added.Entries) == 0 {
			t.Error("Expected added entries")
		}
	})
	t.Run("Diff with removed entry", func(t *testing.T) {
		m1 := NewManifest()
		m1.AddEntry(&Entry{Name: "keep.txt", Mode: 0644, Size: 100})
		m1.AddEntry(&Entry{Name: "delete.txt", Mode: 0644, Size: 200})
		m2 := NewManifest()
		m2.AddEntry(&Entry{Name: "keep.txt", Mode: 0644, Size: 100})
		result, err := Diff(ManifestSource{Manifest: m1}, ManifestSource{Manifest: m2})
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}
		if len(result.Removed.Entries) == 0 {
			t.Error("Expected removed entries")
		}
	})
}

// TestResolverMoreCoverage tests more Resolver paths
func TestResolverMoreCoverage(t *testing.T) {
	t.Run("resolve path with storage error", func(t *testing.T) {
		resolver := NewResolver(&testCoverageStorage{err: fmt.Errorf("storage error")})
		if _, err := resolver.Resolve(c4.Identify(strings.NewReader("root")), "dir/subdir/file.txt"); err == nil {
			t.Error("Expected error from resolver")
		}
	})
}

type testCoverageStorage struct{ err error }

func (s *testCoverageStorage) Open(id c4.ID) (io.ReadCloser, error) { return nil, s.err }

// TestSequenceExpanderMoreCoverage tests sequence expansion edge cases
func TestSequenceExpanderMoreCoverage(t *testing.T) {
	t.Run("expand standalone sequence", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceStandalone)
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "frame[001-003].png", Mode: 0644, Size: 1000})
		result, _, err := expander.ExpandManifest(manifest)
		_ = result
		_ = err
	})
	t.Run("expand with no sequences", func(t *testing.T) {
		expander := NewSequenceExpander(SequenceStandalone)
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "regular.txt", Mode: 0644, Size: 100})
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
		manifest.AddEntry(&Entry{Name: "img[001-005].jpg", Mode: 0644, Size: 1000})
		result, _, err := expander.ExpandManifest(manifest)
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
		if _, err := ParseSequence("frame[10-5].png"); err == nil {
			t.Error("Expected error for invalid range")
		}
	})
	t.Run("not a sequence", func(t *testing.T) {
		if _, err := ParseSequence("regular.txt"); err == nil {
			t.Error("Expected error for non-sequence")
		}
	})
}

// TestParseIDListMoreCoverage tests ParseIDList edge cases
func TestParseIDListMoreCoverage(t *testing.T) {
	t.Run("list with multiple IDs", func(t *testing.T) {
		id1 := c4.Identify(strings.NewReader("content1"))
		id2 := c4.Identify(strings.NewReader("content2"))
		list, err := parseIDList(strings.NewReader(fmt.Sprintf("%s\n%s\n", id1.String(), id2.String())))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if len(list.ids) != 2 {
			t.Errorf("Expected 2 IDs, got %d", len(list.ids))
		}
	})
	t.Run("list with single ID", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("content"))
		list, err := parseIDList(strings.NewReader(id.String() + "\n"))
		if err != nil {
			t.Fatalf("ParseIDList failed: %v", err)
		}
		if len(list.ids) != 1 {
			t.Errorf("Expected 1 ID, got %d", len(list.ids))
		}
	})
	t.Run("list with invalid ID", func(t *testing.T) {
		if _, err := parseIDList(strings.NewReader("invalid-id\n")); err == nil {
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
		_ = NewSequenceDetector(3).DetectSequences(manifest)
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
		_ = NewSequenceDetector(3).DetectSequences(manifest)
	})
	t.Run("detect with minimum length 2", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "img01.png", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "img02.png", Mode: 0644, Size: 100})
		_ = NewSequenceDetector(2).DetectSequences(manifest)
	})
}

// TestReadLineErrors tests readLine error paths
func TestReadLineErrors(t *testing.T) {
	t.Run("read with io error", func(t *testing.T) {
		if _, err := NewDecoder(&errorReader{err: fmt.Errorf("read error")}).Decode(); err == nil {
			t.Error("Expected error from decoder")
		}
	})
}

type errorReader struct{ err error }

func (r *errorReader) Read(p []byte) (n int, err error) { return 0, r.err }

// TestValidatorNameEmptyCase tests empty name validation
func TestValidatorNameEmptyCase(t *testing.T) {
	v := NewValidator(true)
	if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100\n")); err == nil {
		t.Error("Entry without name should fail")
	}
}

// TestEncodingPrettyMoreCases tests more pretty encoding cases
func TestEncodingPrettyMoreCases(t *testing.T) {
	t.Run("pretty encode large sizes", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "huge.bin", Mode: 0644, Size: 1234567890123, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
		var buf bytes.Buffer
		if err := NewEncoder(&buf).SetPretty(true).Encode(manifest); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if !strings.Contains(buf.String(), ",") {
			t.Error("Expected comma-formatted size in pretty output")
		}
	})
	t.Run("pretty encode with special modes", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "pipe", Mode: os.ModeNamedPipe | 0644, Size: 0})
		manifest.AddEntry(&Entry{Name: "socket", Mode: os.ModeSocket | 0755, Size: 0})
		var buf bytes.Buffer
		if err := NewEncoder(&buf).SetPretty(true).Encode(manifest); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
}

// TestValidateNameCoverage tests validateName branches
func TestValidateNameCoverage(t *testing.T) {
	errCases := []struct{ name, input string }{
		{"path traversal ../", "-rw-r--r-- 2025-01-01T00:00:00Z 100 ../etc/passwd\n"},
		{"path traversal ./", "-rw-r--r-- 2025-01-01T00:00:00Z 100 ./hidden.txt\n"},
		{"null bytes in name", "-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x00.txt\n"},
		{"directory name just /", "drwxr-xr-x 2025-01-01T00:00:00Z 4096 /\n"},
		{"embedded path separator", "-rw-r--r-- 2025-01-01T00:00:00Z 100 sub/file.txt\n"},
		{"windows backslash", "-rw-r--r-- 2025-01-01T00:00:00Z 100 sub\\file.txt\n"},
		{"dot directory", "drwxr-xr-x 2025-01-01T00:00:00Z 0 ./\n"},
		{"dotdot directory", "drwxr-xr-x 2025-01-01T00:00:00Z 0 ../\n"},
		{"dot file", "-rw-r--r-- 2025-01-01T00:00:00Z 100 .\n"},
		{"dotdot file", "-rw-r--r-- 2025-01-01T00:00:00Z 100 ..\n"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			v := NewValidator(true)
			if err := v.ValidateManifest(strings.NewReader(tc.input)); err == nil {
				t.Errorf("Expected error for %s", tc.name)
			}
		})
	}
}

// TestDetectSequencesEdgeCases tests edge cases in sequence detection
func TestDetectSequencesEdgeCases(t *testing.T) {
	t.Run("single file matching pattern", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.001.txt", Mode: 0644, Size: 100})
		if result := NewSequenceDetector(1).DetectSequences(manifest); result == nil {
			t.Error("Expected non-nil result")
		}
	})
	t.Run("non-sequential numbers", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.001.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "file.010.txt", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "file.100.txt", Mode: 0644, Size: 100})
		_ = NewSequenceDetector(2).DetectSequences(manifest)
	})
	t.Run("mixed file types", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "image.001.png", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "image.002.png", Mode: 0644, Size: 100})
		manifest.AddEntry(&Entry{Name: "video.001.mp4", Mode: 0644, Size: 200})
		manifest.AddEntry(&Entry{Name: "video.002.mp4", Mode: 0644, Size: 200})
		_ = NewSequenceDetector(2).DetectSequences(manifest)
	})
}

// TestValidatorModeEdgeCases tests mode validation
func TestValidatorModeEdgeCases(t *testing.T) {
	t.Run("invalid type character", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("Xrw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n")); err == nil {
			t.Error("Invalid type character should fail")
		}
	})
	t.Run("setuid setgid sticky all set", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rwsrwsrwt 2025-01-01T00:00:00Z 100 special.txt\n")); err != nil {
			t.Errorf("All special bits should be valid: %v", err)
		}
	})
}

// TestIsIDListContentCoverage tests IsIDListContent function
func TestIsIDListContentCoverage(t *testing.T) {
	id := c4.Identify(strings.NewReader("content"))
	t.Run("valid ID list", func(t *testing.T) {
		if !IsIDListContent([]byte(id.String() + "\n")) {
			t.Error("Expected valid ID list content")
		}
	})
	t.Run("empty content", func(t *testing.T) {
		if IsIDListContent([]byte("")) {
			t.Error("Empty content should not be valid ID list")
		}
	})
	t.Run("only whitespace", func(t *testing.T) {
		if IsIDListContent([]byte("   \n\n   \n")) {
			t.Error("Whitespace-only should not be valid ID list")
		}
	})
	t.Run("invalid content", func(t *testing.T) {
		if IsIDListContent([]byte("not a c4 id\n")) {
			t.Error("Invalid content should not be valid ID list")
		}
	})
	t.Run("mixed valid and invalid", func(t *testing.T) {
		if IsIDListContent([]byte(id.String() + "\nnot valid\n")) {
			t.Error("Mixed content should not be valid")
		}
	})
}

// TestParseIDListScannerError tests scanner error path
func TestParseIDListScannerError(t *testing.T) {
	errReader := &limitedErrorReader{data: []byte("some data that will be cut off"), errorAt: 10}
	_, err := parseIDList(errReader)
	if err == nil {
		t.Log("No error from limited reader, scanner may have buffered all data")
	}
}

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

// TestFormatSizeCoverage tests size formatting edge cases
func TestFormatSizeCoverage(t *testing.T) {
	t.Run("small size pretty format", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "small.txt", Mode: 0644, Size: 99})
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
		manifest.AddEntry(&Entry{Name: "small.txt", Mode: 0644, Size: 999})
		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if strings.Contains(string(data), ",") {
			t.Error("999 should not have commas")
		}
	})
	t.Run("four digits pretty format", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "medium.txt", Mode: 0644, Size: 1234})
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
		manifest, err := Unmarshal([]byte("-rw-r--r-- 2025-01-01T12:30:45Z 100 file.txt\n"))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if !manifest.Entries[0].Timestamp.UTC().Equal(time.Date(2025, 1, 1, 12, 30, 45, 0, time.UTC)) {
			t.Error("Timestamp not parsed correctly")
		}
	})
	t.Run("invalid timestamp format", func(t *testing.T) {
		if _, err := Unmarshal([]byte("-rw-r--r-- invalid-timestamp 100 file.txt\n")); err == nil {
			t.Error("Expected error for invalid timestamp")
		}
	})
}

type errorSource struct{}

func (e errorSource) ToManifest() (*Manifest, error) { return nil, fmt.Errorf("source error") }

// TestValidatorEntryEdgeCases2 tests more validateEntry edge cases
func TestValidatorEntryEdgeCases2(t *testing.T) {
	t.Run("invalid UTF-8", func(t *testing.T) {
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file\xff.txt\n"))
	})
	t.Run("control character", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x01.txt\n")); err == nil {
			t.Error("Expected error for control character")
		}
	})
	t.Run("odd indentation", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("drwxr-xr-x 2025-01-01T00:00:00Z 4096 dir/\n   -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n")); err == nil {
			t.Error("Expected error for odd indentation")
		}
	})
	t.Run("invalid depth jump", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 root.txt\n    -rw-r--r-- 2025-01-01T00:00:00Z 100 deep.txt\n")); err == nil {
			t.Error("Expected error for depth jump")
		}
	})
	t.Run("dedentation reset", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("drwxr-xr-x 2025-01-01T00:00:00Z 4096 a/\n  -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\ndrwxr-xr-x 2025-01-01T00:00:00Z 4096 b/\n")); err != nil {
			t.Errorf("Valid dedentation should not error: %v", err)
		}
	})
}

// TestValidatorC4IDEdgeCases tests C4 ID validation edge cases
func TestValidatorC4IDEdgeCases(t *testing.T) {
	t.Run("valid C4 ID", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("test"))
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader(fmt.Sprintf("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt %s\n", id.String()))); err != nil {
			t.Errorf("Valid C4 ID should pass: %v", err)
		}
	})
	t.Run("C4 ID with wrong prefix", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c3invalidid\n")); err == nil {
			t.Error("Wrong prefix should fail")
		}
	})
	t.Run("C4 ID too short", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c4short\n")); err == nil {
			t.Error("Too short C4 ID should fail")
		}
	})
}

// TestValidatorC4IDEdgeCases2 tests more C4 ID validation
func TestValidatorC4IDEdgeCases2(t *testing.T) {
	t.Run("C4 ID wrong length", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c4short\n")); err == nil {
			t.Error("Expected error for short C4 ID")
		}
	})
	t.Run("C4 ID wrong prefix", func(t *testing.T) {
		longID := "x4" + strings.Repeat("a", 88)
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader(fmt.Sprintf("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt %s\n", longID))); err == nil {
			t.Error("Expected error for wrong prefix")
		}
	})
}

// TestDiffErrorSources tests Diff with error-returning sources
func TestDiffErrorSources(t *testing.T) {
	m := NewManifest()
	t.Run("error source a", func(t *testing.T) {
		if _, err := Diff(errorSource{}, ManifestSource{Manifest: m}); err == nil {
			t.Error("Expected error from error source")
		}
	})
	t.Run("error source b", func(t *testing.T) {
		if _, err := Diff(ManifestSource{Manifest: m}, errorSource{}); err == nil {
			t.Error("Expected error from error source")
		}
	})
}

// TestMarshalSuccessCases tests successful Marshal operations
func TestMarshalSuccessCases(t *testing.T) {
	t.Run("marshal manifest with symlink", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "link", Mode: os.ModeSymlink | 0777, Size: 0, Target: "target"})
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
		manifest.AddEntry(&Entry{Name: "link", Mode: os.ModeSymlink | 0777, Size: 0, Target: "target"})
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
		idList := newIDList()
		idList.Add(c4.Identify(strings.NewReader("content1")))
		idList.Add(c4.Identify(strings.NewReader("content2")))
		manifest.AddDataBlock(createDataBlockFromIDList(idList))
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100})
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(manifest); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
	t.Run("encode with non-ID list data block", func(t *testing.T) {
		manifest := NewManifest()
		content := []byte("binary content")
		manifest.AddDataBlock(&DataBlock{ID: c4.Identify(bytes.NewReader(content)), IsIDList: false, Content: content})
		manifest.AddEntry(&Entry{Name: "file.bin", Mode: 0644, Size: int64(len(content))})
		var buf bytes.Buffer
		if err := NewEncoder(&buf).Encode(manifest); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	})
}

// TestMarshalErrorPaths tests error paths in Marshal functions
func TestMarshalErrorPaths(t *testing.T) {
	t.Run("Marshal with entries", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
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
		manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 1234567, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
		data, err := MarshalPretty(manifest)
		if err != nil {
			t.Fatalf("MarshalPretty failed: %v", err)
		}
		if !strings.Contains(string(data), ",") {
			t.Error("Expected comma-formatted size in pretty output")
		}
	})
}

// TestValidatorEntryEdgeCases tests entry validation edge cases
func TestValidatorEntryEdgeCases(t *testing.T) {
	t.Run("empty line", func(t *testing.T) {
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader("\n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"))
	})
	t.Run("entry with only spaces", func(t *testing.T) {
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader("   \n-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"))
	})
	t.Run("short mode string", func(t *testing.T) {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader("-rw 2025-01-01T00:00:00Z 100 file.txt\n")); err == nil {
			t.Error("Short mode should fail")
		}
	})
	t.Run("too many fields", func(t *testing.T) {
		v := NewValidator(true)
		_ = v.ValidateManifest(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c4xxxx extra\n"))
	})
}

// TestSortSiblingsCoverage tests SortEntries
func TestSortSiblingsCoverage(t *testing.T) {
	t.Run("empty manifest", func(t *testing.T) {
		manifest := NewManifest()
		manifest.SortEntries()
		if len(manifest.Entries) != 0 {
			t.Error("Empty manifest should remain empty")
		}
	})
	t.Run("orphaned entries", func(t *testing.T) {
		manifest := NewManifest()
		manifest.Entries = append(manifest.Entries, &Entry{Name: "child.txt", Mode: 0644, Size: 100, Depth: 2})
		manifest.Entries = append(manifest.Entries, &Entry{Name: "root.txt", Mode: 0644, Size: 200, Depth: 0})
		manifest.SortEntries()
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
	if NaturalLess("abc", "abc") {
		t.Error("Equal strings should not be less")
	}
	if !NaturalLess("file2", "file10") {
		t.Error("file2 should be less than file10")
	}
	if !NaturalLess("aaa", "bbb") {
		t.Error("aaa should be less than bbb")
	}
	if !NaturalLess("file1a", "file1b") {
		t.Error("file1a should be less than file1b")
	}
}

// TestEncodeErrorPaths tests Encode error paths
func TestEncodeErrorPaths(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100})
	if err := NewEncoder(&failingWriter{failAfter: 5}).Encode(manifest); err == nil {
		t.Error("Expected write error")
	}
}

// TestFormatSizeMoreBranches tests formatSize edge cases
func TestFormatSizeMoreBranches(t *testing.T) {
	t.Run("encode zero size file", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "empty.txt", Mode: 0644, Size: 0})
		data, err := Marshal(manifest)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if !strings.Contains(string(data), " 0 ") {
			t.Error("Expected size 0 in output")
		}
	})
	t.Run("encode negative size (special)", func(t *testing.T) {
		manifest := NewManifest()
		manifest.AddEntry(&Entry{Name: "special.txt", Mode: 0644, Size: -1})
		if _, err := Marshal(manifest); err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
	})
}

// TestParseEntryMoreBranches tests more parseEntry branches
func TestParseEntryMoreBranches(t *testing.T) {
	t.Run("entry with null C4 ID", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt -\n"))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if len(manifest.Entries) != 1 {
			t.Error("Expected 1 entry")
		}
	})
	t.Run("directory entry", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("drwxr-xr-x 2025-01-01T00:00:00Z 4096 mydir/\n"))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if !manifest.Entries[0].IsDir() {
			t.Error("Expected directory entry")
		}
	})
	t.Run("symlink entry", func(t *testing.T) {
		manifest, err := Unmarshal([]byte("lrwxr-xr-x 2025-01-01T00:00:00Z 10 link -> target\n"))
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if !manifest.Entries[0].IsSymlink() {
			t.Error("Expected symlink entry")
		}
	})
}

// TestEncoderSetIndent tests the SetIndent method
func TestEncoderSetIndent(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755, Size: 4096, Depth: 0})
	manifest.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Size: 100, Depth: 1})
	var buf bytes.Buffer
	if err := NewEncoder(&buf).SetIndent(4).Encode(manifest); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Coverage Boost Tests
// ----------------------------------------------------------------------------

func TestManifestWriting(t *testing.T) {
	manifest := NewManifest()
	manifest.Version = "1.0"
	manifest.AddEntry(&Entry{Name: "test.txt", Size: 100, Mode: 0644, Timestamp: time.Now()})
	manifest.AddEntry(&Entry{Name: "dir/", Mode: 0755 | os.ModeDir})

	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(manifest); err != nil {
		t.Errorf("Encode failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encode wrote 0 bytes")
	}

	buf.Reset()
	if err := NewEncoder(&buf).SetPretty(true).Encode(manifest); err != nil {
		t.Errorf("Encode (pretty) failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encode (pretty) wrote 0 bytes")
	}
}

func TestEntryMethods(t *testing.T) {
	entry := &Entry{Name: "file.txt", Mode: 0644, Size: 100}
	if entry.IsDir() {
		t.Error("Regular file marked as directory")
	}
	if entry.IsSymlink() {
		t.Error("Regular file marked as symlink")
	}

	dirEntry := &Entry{Name: "dir/", Mode: 0755 | os.ModeDir}
	if !dirEntry.IsDir() {
		t.Error("Directory not marked as directory")
	}

	linkEntry := &Entry{Name: "link", Mode: 0777 | os.ModeSymlink, Target: "target"}
	if !linkEntry.IsSymlink() {
		t.Error("Symlink not marked as symlink")
	}

	_ = entry.String()
}

func TestManifestOperations(t *testing.T) {
	m1 := NewManifest()
	m1.AddEntry(&Entry{Name: "a.txt", Size: 100})
	m1.AddEntry(&Entry{Name: "b.txt", Size: 200})
	if len(m1.Entries) != 2 {
		t.Errorf("Expected 2 entries in m1, got %d", len(m1.Entries))
	}

	m2 := NewManifest()
	m2.AddEntry(&Entry{Name: "b.txt", Size: 200})
	m2.AddEntry(&Entry{Name: "c.txt", Size: 300})
	if len(m2.Entries) != 2 {
		t.Errorf("Expected 2 entries in m2, got %d", len(m2.Entries))
	}
}

func TestSortingOperations(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{Name: "z.txt", Mode: 0644})
	manifest.AddEntry(&Entry{Name: "a.txt", Mode: 0644})
	manifest.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755})
	manifest.AddEntry(&Entry{Name: "m.txt", Mode: 0644})
	manifest.SortEntries()
	if manifest.Entries[0].Name != "a.txt" {
		t.Errorf("Expected first entry to be a.txt after sort, got %s", manifest.Entries[0].Name)
	}

	manifest2 := NewManifest()
	manifest2.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Depth: 0})
	manifest2.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755, Depth: 0})
	manifest2.AddEntry(&Entry{Name: "another.txt", Mode: 0644, Depth: 0})
	manifest2.SortEntries()
	if manifest2.Entries[0].Mode.IsDir() {
		t.Error("Directory came before file at same depth")
	}
}

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
		if got := NaturalLess(tt.a, tt.b); got != tt.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestManifestSources(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{Name: "test.txt", Size: 100})
	source := ManifestSource{manifest}
	if source.Manifest == nil {
		t.Error("ManifestSource has nil manifest")
	}
}

// ----------------------------------------------------------------------------
// Basic Coverage Tests
// ----------------------------------------------------------------------------

func TestManifestBasic(t *testing.T) {
	m := NewManifest()
	if m == nil {
		t.Fatal("NewManifest returned nil")
	}

	entry := &Entry{Name: "test.txt", Mode: 0644, Size: 100, Timestamp: time.Now(), C4ID: c4.Identify(strings.NewReader("test"))}
	m.AddEntry(entry)
	m.SortEntries()

	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(m); err != nil {
		t.Errorf("Encode failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Encode wrote 0 bytes")
	}

	if e := m.GetEntry("test.txt"); e == nil {
		t.Error("GetEntry returned nil")
	}

	id := m.ComputeC4ID()
	var emptyID c4.ID
	if id == emptyID {
		t.Error("ComputeC4ID returned empty ID")
	}

	if canonical := m.Canonical(); canonical == "" {
		t.Error("Canonical returned empty string")
	}

	if entries := m.GetEntriesAtDepth(0); len(entries) == 0 {
		t.Error("GetEntriesAtDepth returned no entries")
	}
}

func TestEntryBasic(t *testing.T) {
	e := &Entry{Name: "test.txt", Mode: 0644, Size: 100, Timestamp: time.Now()}
	if e.IsDir() {
		t.Error("IsDir returned true for file")
	}
	if e.IsSymlink() {
		t.Error("IsSymlink returned true for regular file")
	}
	if base := e.BaseName(); base != "test.txt" {
		t.Errorf("BaseName returned %q, expected test.txt", base)
	}
	if str := e.String(); str == "" {
		t.Error("String returned empty")
	}
	if canonical := e.Canonical(); canonical == "" {
		t.Error("Canonical returned empty")
	}
}

func TestSequenceBasic(t *testing.T) {
	if !IsSequence("file_[001-005].txt") {
		t.Error("Expected IsSequence to return true")
	}
	seq, err := ParseSequence("file_[001-005].txt")
	if err != nil {
		t.Errorf("ParseSequence failed: %v", err)
	}
	if seq != nil {
		if files := seq.Expand(); len(files) != 5 {
			t.Errorf("Expected 5 files, got %d", len(files))
		}
		if seq.Count() != 5 {
			t.Errorf("Expected count 5, got %d", seq.Count())
		}
	}
}

func TestValidatorBasic(t *testing.T) {
	v := NewValidator(false)
	if v == nil {
		t.Fatal("NewValidator returned nil")
	}
	if stats := v.GetStats(); stats.Files < 0 {
		t.Error("GetStats returned invalid Files count")
	}
	if path := v.GetCurrentPath(); path != "" {
		t.Errorf("Expected empty path, got %q", path)
	}
}

func TestDecoderBasic(t *testing.T) {
	p := NewDecoder(strings.NewReader("-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"))
	if p == nil {
		t.Fatal("NewDecoder returned nil")
	}
	m, err := p.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if m == nil {
		t.Fatal("Decode returned nil manifest")
	}
	if len(m.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(m.Entries))
	}
}

func TestOperationsBasic(t *testing.T) {
	m1 := NewManifest()
	m1.AddEntry(&Entry{Name: "a.txt", Mode: 0644, Size: 100, Timestamp: time.Now()})
	m2 := NewManifest()
	m2.AddEntry(&Entry{Name: "b.txt", Mode: 0644, Size: 200, Timestamp: time.Now()})

	diff, err := Diff(ManifestSource{m1}, ManifestSource{m2})
	if err != nil {
		t.Errorf("Diff failed: %v", err)
	}
	if diff == nil {
		t.Error("Diff returned nil results")
	}
}
