package c4m

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDecoder_parseEntry(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Entry
		wantErr bool
	}{
		{
			name:  "regular_file",
			input: "-rw-r--r-- 2023-01-01T12:00:00Z 1024 README.md c43zYcLni5LF9rR4Lg4B8h3Jp8SBwjcnyyeh4bc6gTPHndKuKdjUWx1kJPYhZxYt3zV6tQXpDs2shPsPYjgG81wZM1\n",
			want: &Entry{
				Mode:      0644,
				Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				Size:      1024,
				Name:      "README.md",
				Depth:     0,
			},
			wantErr: false,
		},
		{
			name:  "directory",
			input: "drwxr-xr-x 2023-01-01T12:00:00Z 4096 src/\n",
			want: &Entry{
				Mode:      os.FileMode(0755) | os.ModeDir,
				Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				Size:      4096,
				Name:      "src/",
				Depth:     0,
			},
			wantErr: false,
		},
		{
			name:  "symlink",
			input: "lrwxrwxrwx 2023-01-01T12:00:00Z 0 link.txt -> target.txt\n",
			want: &Entry{
				Mode:      os.FileMode(0777) | os.ModeSymlink,
				Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				Size:      0,
				Name:      "link.txt",
				Target:    "target.txt",
				Depth:     0,
			},
			wantErr: false,
		},
		{
			name:  "file_with_spaces",
			input: "-rw-r--r-- 2023-01-01T12:00:00Z 2048 my\\ file.txt\n",
			want: &Entry{
				Mode:      0644,
				Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				Size:      2048,
				Name:      "my file.txt",
				Depth:     0,
			},
			wantErr: false,
		},
		{
			name:  "indented_entry",
			input: "  -rw-r--r-- 2023-01-01T12:00:00Z 512 nested.txt\n",
			want: &Entry{
				Mode:      0644,
				Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				Size:      512,
				Name:      "nested.txt",
				Depth:     1,
			},
			wantErr: false,
		},
		{
			name:    "invalid_mode",
			input:   "invalid 2023-01-01T12:00:00Z 1024 file.txt\n",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid_timestamp",
			input:   "-rw-r--r-- not-a-timestamp 1024 file.txt\n",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid_size",
			input:   "-rw-r--r-- 2023-01-01T12:00:00Z not-a-size file.txt\n",
			want:    nil,
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewDecoder(strings.NewReader(tt.input))

			// Parse entry
			got, err := parser.parseEntry()
			
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEntry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr {
				// Compare key fields
				if got.Mode != tt.want.Mode {
					t.Errorf("Mode = %v, want %v", got.Mode, tt.want.Mode)
				}
				if !got.Timestamp.Equal(tt.want.Timestamp) {
					t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.want.Timestamp)
				}
				if got.Size != tt.want.Size {
					t.Errorf("Size = %v, want %v", got.Size, tt.want.Size)
				}
				if got.Name != tt.want.Name {
					t.Errorf("Name = %v, want %v", got.Name, tt.want.Name)
				}
				if got.Target != tt.want.Target {
					t.Errorf("Target = %v, want %v", got.Target, tt.want.Target)
				}
				if got.Depth != tt.want.Depth {
					t.Errorf("Depth = %v, want %v", got.Depth, tt.want.Depth)
				}
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    os.FileMode
		wantErr bool
	}{
		{"regular_644", "-rw-r--r--", 0644, false},
		{"regular_755", "-rwxr-xr-x", 0755, false},
		{"directory", "drwxr-xr-x", 0755 | os.ModeDir, false},
		{"symlink", "lrwxrwxrwx", 0777 | os.ModeSymlink, false},
		{"setuid", "-rwsr-xr-x", 0755 | os.ModeSetuid, false},
		{"setgid", "-rwxr-sr-x", 0755 | os.ModeSetgid, false},
		{"sticky", "drwxrwxrwt", 0777 | os.ModeDir | os.ModeSticky, false},
		{"invalid_length", "drwxr-xr", 0, true},
		{"invalid_type", "xrwxr-xr-x", 0, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMode(tt.input)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, m *Manifest)
	}{
		{
			name:  "basic manifest",
			input: `-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 1 {
					t.Fatalf("Entries = %d, want 1", len(m.Entries))
				}
				if m.Entries[0].Name != "file.txt" {
					t.Errorf("Name = %q, want file.txt", m.Entries[0].Name)
				}
			},
		},
		{
			name:  "empty manifest",
			input: ``,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 0 {
					t.Errorf("Entries = %d, want 0", len(m.Entries))
				}
			},
		},
		{
			name: "multiple entries",
			input: `-rw-r--r-- 2024-01-01T00:00:00Z 100 file1.txt
-rw-r--r-- 2024-01-01T00:00:00Z 200 file2.txt
drwxr-xr-x 2024-01-01T00:00:00Z 0 dir/`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 3 {
					t.Fatalf("Entries = %d, want 3", len(m.Entries))
				}
			},
		},
		{
			name:  "symlink entry",
			input: `lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> target`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 1 {
					t.Fatalf("Expected 1 entry, got %d", len(m.Entries))
				}
				e := m.Entries[0]
				if !e.IsSymlink() {
					t.Error("Entry should be a symlink")
				}
				if e.Name != "link" {
					t.Errorf("Name = %q, want %q", e.Name, "link")
				}
				if e.Target != "target" {
					t.Errorf("Target = %q, want %q", e.Target, "target")
				}
			},
		},
		{
			name:  "symlink with spaces in target",
			input: `lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> target\ with\ spaces`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 1 {
					t.Fatalf("Expected 1 entry, got %d", len(m.Entries))
				}
				e := m.Entries[0]
				if !e.IsSymlink() {
					t.Error("Entry should be a symlink")
				}
				if e.Target != "target with spaces" {
					t.Errorf("Target = %q, want %q", e.Target, "target with spaces")
				}
			},
		},
		{
			name:  "symlink to absolute path",
			input: `lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> /absolute/path/target`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 1 {
					t.Fatalf("Expected 1 entry, got %d", len(m.Entries))
				}
				e := m.Entries[0]
				if !e.IsSymlink() {
					t.Error("Entry should be a symlink")
				}
				if e.Target != "/absolute/path/target" {
					t.Errorf("Target = %q, want %q", e.Target, "/absolute/path/target")
				}
			},
		},
		{
			name:    "directive line is error",
			input:   `@base c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewDecoder(strings.NewReader(tt.input))
			m, err := p.Decode()
			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, m)
			}
		})
	}
}

func TestDecoder_parseEntryEOF(t *testing.T) {
	p := NewDecoder(strings.NewReader(""))

	_, err := p.parseEntry()
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestParserNew(t *testing.T) {
	// Test parsing a complete manifest
	content := `-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
drwxr-xr-x 2025-09-19T12:00:00Z 200 dir/
  -rw-r--r-- 2025-09-19T12:00:00Z 200 file.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	parser := NewDecoder(strings.NewReader(content))

	manifest, err := parser.Decode()
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Check number of entries
	if len(manifest.Entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(manifest.Entries))
	}

	// Verify entries
	if manifest.Entries[0].Name != "test.txt" {
		t.Errorf("First entry should be test.txt, got %s", manifest.Entries[0].Name)
	}
	if manifest.Entries[1].Name != "dir/" {
		t.Errorf("Second entry should be dir/, got %s", manifest.Entries[1].Name)
	}
	if manifest.Entries[2].Name != "file.txt" {
		t.Errorf("Third entry should be file.txt (inside dir), got %s", manifest.Entries[2].Name)
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantUTC string // Expected time in UTC
		wantErr bool
	}{
		{
			name:    "canonical UTC format",
			input:   "2024-06-15T10:30:00Z",
			wantUTC: "2024-06-15T10:30:00Z",
			wantErr: false,
		},
		{
			name:    "RFC3339 with positive offset",
			input:   "2024-06-15T10:30:00+05:00",
			wantUTC: "2024-06-15T05:30:00Z", // Converted to UTC
			wantErr: false,
		},
		{
			name:    "RFC3339 with negative offset",
			input:   "2024-06-15T10:30:00-07:00",
			wantUTC: "2024-06-15T17:30:00Z", // Converted to UTC
			wantErr: false,
		},
		{
			name:    "Unix date format",
			input:   "Sat Jun 15 10:30:00 UTC 2024",
			wantUTC: "2024-06-15T10:30:00Z",
			wantErr: false,
		},
		{
			name:    "pretty format with timezone",
			input:   "Jun 15 10:30:00 2024 UTC",
			wantUTC: "2024-06-15T10:30:00Z",
			wantErr: false,
		},
		{
			name:    "pretty format single digit day",
			input:   "Jun  5 10:30:00 2024 UTC",
			wantUTC: "2024-06-05T10:30:00Z",
			wantErr: false,
		},
		{
			name:    "pretty format with numeric offset",
			input:   "Jun 15 10:30:00 2024 -0700",
			wantUTC: "2024-06-15T17:30:00Z",
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "not a timestamp",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimestamp(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseTimestamp() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("parseTimestamp() error = %v", err)
				return
			}
			gotStr := got.Format(TimestampFormat)
			if gotStr != tt.wantUTC {
				t.Errorf("parseTimestamp() = %v, want %v", gotStr, tt.wantUTC)
			}
		})
	}
}

func TestEscapedBracketsNotSequence(t *testing.T) {
	// A name with escaped brackets must NOT be treated as a sequence
	input := "-rw-r--r-- 2024-01-01T00:00:00Z 100 render\\[v2\\].exr\n"
	manifest, err := NewDecoder(strings.NewReader(input)).Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(manifest.Entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(manifest.Entries))
	}
	entry := manifest.Entries[0]
	if entry.Name != "render[v2].exr" {
		t.Errorf("Name = %q, want %q", entry.Name, "render[v2].exr")
	}
	if entry.IsSequence {
		t.Error("Escaped brackets should not be flagged as sequence")
	}
}

func TestUnquotedSequenceStillDetected(t *testing.T) {
	// An unquoted name with brackets should still be a sequence
	input := "-rw-r--r-- 2024-01-01T00:00:00Z 100 render.[001-010].exr\n"
	manifest, err := NewDecoder(strings.NewReader(input)).Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(manifest.Entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(manifest.Entries))
	}
	entry := manifest.Entries[0]
	if !entry.IsSequence {
		t.Error("Unquoted name with brackets should be flagged as sequence")
	}
}

