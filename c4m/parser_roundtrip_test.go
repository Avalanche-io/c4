package c4m

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestParserRoundTrip(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	testID, _ := c4.Parse("c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")

	tests := []struct {
		name     string
		manifest *Manifest
	}{
		{
			name: "simple manifest",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      100,
						Name:      "file.txt",
						C4ID:      testID,
						Depth:     0,
					},
				},
			},
		},
		{
			name: "manifest with large sizes",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      1234567890,
						Name:      "large.bin",
						C4ID:      testID,
						Depth:     0,
					},
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      42,
						Name:      "small.txt",
						C4ID:      testID,
						Depth:     0,
					},
				},
			},
		},
		{
			name: "manifest with nested structure",
			manifest: &Manifest{
				Version: "1.0",
				Entries: []*Entry{
					{
						Mode:      0755 | 0x40000000, // Directory
						Timestamp: testTime,
						Size:      4096,
						Name:      "dir1/",
						C4ID:      testID,
						Depth:     0,
					},
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      100,
						Name:      "file1.txt",
						C4ID:      testID,
						Depth:     1,
					},
					{
						Mode:      0755 | 0x40000000, // Directory
						Timestamp: testTime,
						Size:      4096,
						Name:      "dir2/",
						C4ID:      testID,
						Depth:     1,
					},
					{
						Mode:      0644,
						Timestamp: testTime,
						Size:      200,
						Name:      "file2.txt",
						C4ID:      testID,
						Depth:     2,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test canonical format round-trip
			t.Run("canonical", func(t *testing.T) {
				var buf bytes.Buffer
				err := NewEncoder(&buf).Encode(tt.manifest)
				if err != nil {
					t.Fatalf("Encode() error = %v", err)
				}

				// Parse back
				parsed, err := NewDecoder(&buf).Decode()
				if err != nil {
					t.Fatalf("NewDecoder() error = %v", err)
				}

				// Compute C4 IDs
				originalID := tt.manifest.ComputeC4ID()
				parsedID := parsed.ComputeC4ID()

				if originalID != parsedID {
					t.Errorf("C4 ID mismatch: original=%s, parsed=%s", originalID, parsedID)
				}
			})

			// Test pretty format round-trip
			t.Run("pretty", func(t *testing.T) {
				var buf bytes.Buffer
				err := NewEncoder(&buf).SetPretty(true).Encode(tt.manifest)
				if err != nil {
					t.Fatalf("Encode (pretty) error = %v", err)
				}

				// Parse back
				parsed, err := NewDecoder(&buf).Decode()
				if err != nil {
					t.Fatalf("NewDecoder() error = %v", err)
				}

				// Compute C4 IDs
				originalID := tt.manifest.ComputeC4ID()
				parsedID := parsed.ComputeC4ID()

				if originalID != parsedID {
					t.Errorf("C4 ID mismatch: original=%s, parsed=%s", originalID, parsedID)
				}
			})
		})
	}
}

func TestParserErgonomicForms(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		checkSize   int64
	}{
		{
			name: "size with commas",
			input: `-rw-r--r-- 2024-01-15T10:30:00Z 1,234,567 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: false,
			checkSize:   1234567,
		},
		{
			name: "pretty timestamp with timezone",
			input: `-rw-r--r-- Jan 15 10:30:00 2024 UTC 100 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: false,
			checkSize:   100,
		},
		{
			name: "pretty timestamp with local timezone",
			input: `-rw-r--r-- Sep  1 12:30:00 2024 CDT 100 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: false,
			checkSize:   100,
		},
		{
			name: "size with spaces (padding)",
			input: `-rw-r--r-- 2024-01-15T10:30:00Z       100 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: false,
			checkSize:   100,
		},
		{
			name: "column-aligned C4 ID",
			input: `-rw-r--r-- 2024-01-15T10:30:00Z 100 file.txt                                        c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: false,
			checkSize:   100,
		},
		{
			name: "large number with commas",
			input: `-rw-r--r-- 2024-01-15T10:30:00Z 10,485,760 bigfile.bin c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: false,
			checkSize:   10485760,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := NewDecoder(strings.NewReader(tt.input)).Decode()
			
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("NewDecoder() error = %v", err)
			}

			if len(manifest.Entries) != 1 {
				t.Fatalf("Expected 1 entry, got %d", len(manifest.Entries))
			}

			if manifest.Entries[0].Size != tt.checkSize {
				t.Errorf("Size mismatch: got %d, want %d", manifest.Entries[0].Size, tt.checkSize)
			}
		})
	}
}

func TestParserErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "invalid size (not a number after comma removal)",
			input:       `-rw-r--r-- 2024-01-15T10:30:00Z abc,def file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: true,
		},
		{
			name:        "valid manifest should not error",
			input:       `-rw-r--r-- 2024-01-15T10:30:00Z 100 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDecoder(strings.NewReader(tt.input)).Decode()
			
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}