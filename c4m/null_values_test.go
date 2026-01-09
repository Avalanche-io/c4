package c4m

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestParseNullValues(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkMode os.FileMode
		checkTime time.Time
		checkSize int64
		checkC4ID string // Use string for easier comparison
	}{
		{
			name: "null mode",
			input: `@c4m 1.0
---------- 2024-01-01T00:00:00Z 100 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			checkMode: 0,
			checkTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			checkSize: 100,
			checkC4ID: "c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "null timestamp",
			input: `@c4m 1.0
-rw-r--r-- - 100 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			checkMode: 0644,
			checkTime: time.Unix(0, 0).UTC(), // Unix epoch
			checkSize: 100,
			checkC4ID: "c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "null size",
			input: `@c4m 1.0
-rw-r--r-- 2024-01-01T00:00:00Z - file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB`,
			checkMode: 0644,
			checkTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			checkSize: -1, // Null size indicator
			checkC4ID: "c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "null C4 ID",
			input: `@c4m 1.0
-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt -`,
			checkMode: 0644,
			checkTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			checkSize: 100,
			checkC4ID: "", // Zero value
		},
		{
			name: "all null values except name",
			input: `@c4m 1.0
---------- - - file.txt -`,
			checkMode: 0,
			checkTime: time.Unix(0, 0).UTC(),
			checkSize: -1,
			checkC4ID: "",
		},
		{
			name: "single dash mode",
			input: `@c4m 1.0
- 2024-01-01T00:00:00Z 100 file.txt`,
			checkMode: 0,
			checkTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			checkSize: 100,
			checkC4ID: "",
		},
		{
			name: "zero timestamp",
			input: `@c4m 1.0
-rw-r--r-- 0 100 file.txt`,
			checkMode: 0644,
			checkTime: time.Unix(0, 0).UTC(),
			checkSize: 100,
			checkC4ID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := GenerateFromReader(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("GenerateFromReader() error = %v", err)
			}

			if len(manifest.Entries) != 1 {
				t.Fatalf("Expected 1 entry, got %d", len(manifest.Entries))
			}

			entry := manifest.Entries[0]

			// Check mode
			if entry.Mode != tt.checkMode {
				t.Errorf("Mode = %v, want %v", entry.Mode, tt.checkMode)
			}

			// Check timestamp
			if !entry.Timestamp.Equal(tt.checkTime) {
				t.Errorf("Timestamp = %v, want %v", entry.Timestamp, tt.checkTime)
			}

			// Check size
			if entry.Size != tt.checkSize {
				t.Errorf("Size = %d, want %d", entry.Size, tt.checkSize)
			}

			// Check C4 ID
			gotC4ID := entry.C4ID.String()
			if gotC4ID == "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111" {
				gotC4ID = "" // Treat zero C4 ID as empty string
			}
			if gotC4ID != tt.checkC4ID {
				t.Errorf("C4ID = %v, want %v", gotC4ID, tt.checkC4ID)
			}
		})
	}
}

func TestFormatNullValues(t *testing.T) {
	tests := []struct {
		name     string
		entry    *Entry
		contains []string
	}{
		{
			name: "null mode output",
			entry: &Entry{
				Mode:      0,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      100,
				Name:      "file.txt",
			},
			contains: []string{"----------", "2024-01-01T00:00:00Z", "100", "file.txt"},
		},
		{
			name: "null timestamp output",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Unix(0, 0).UTC(),
				Size:      100,
				Name:      "file.txt",
			},
			contains: []string{"-rw-r--r--", "-", "100", "file.txt"},
		},
		{
			name: "null size output",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      -1,
				Name:      "file.txt",
			},
			contains: []string{"-rw-r--r--", "2024-01-01T00:00:00Z", "-", "file.txt"},
		},
		{
			name: "all null values output",
			entry: &Entry{
				Mode:      0,
				Timestamp: time.Unix(0, 0).UTC(),
				Size:      -1,
				Name:      "file.txt",
				C4ID:      c4.ID{},
			},
			contains: []string{"----------", "-", "-", "file.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.entry.Format(0, false)
			
			for _, substr := range tt.contains {
				if !strings.Contains(output, substr) {
					t.Errorf("Output missing %q\nGot: %s", substr, output)
				}
			}
		})
	}
}

func TestNullValueRoundTrip(t *testing.T) {
	// Create manifest with null values
	manifest := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{
				Mode:      0,        // Null mode
				Timestamp: time.Unix(0, 0).UTC(), // Null timestamp
				Size:      -1,       // Null size
				Name:      "test.txt",
				C4ID:      c4.ID{},  // Null C4 ID
				Depth:     0,
			},
			{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      100,
				Name:      "normal.txt",
				Depth:     0,
			},
		},
	}

	// Write to canonical format
	var buf strings.Builder
	err := NewEncoder(&buf).Encode(manifest)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	
	t.Logf("Written manifest:\n%s", buf.String())

	// Parse back
	parsed, err := GenerateFromReader(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("GenerateFromReader() error = %v", err)
	}

	// Verify entries match
	if len(parsed.Entries) != len(manifest.Entries) {
		t.Fatalf("Entry count mismatch: got %d, want %d", len(parsed.Entries), len(manifest.Entries))
	}

	// Find the null value entry (test.txt)
	var nullEntry *Entry
	for _, e := range parsed.Entries {
		if e.Name == "test.txt" {
			nullEntry = e
			break
		}
	}
	if nullEntry == nil {
		t.Fatal("Could not find test.txt in parsed entries")
	}
	
	if nullEntry.Mode != 0 {
		t.Errorf("Null mode not preserved: got %v", nullEntry.Mode)
	}
	if nullEntry.Timestamp.Unix() != 0 {
		t.Errorf("Null timestamp not preserved: got %v", nullEntry.Timestamp)
	}
	if nullEntry.Size != -1 {
		t.Errorf("Null size not preserved: got %d", nullEntry.Size)
	}
	if !nullEntry.C4ID.IsNil() {
		t.Errorf("Null C4ID not preserved: got %v", nullEntry.C4ID)
	}
}