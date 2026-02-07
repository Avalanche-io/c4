package c4m

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestDecoder_parseHeader(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid_v1.0",
			input:   "@c4m 1.0\n",
			want:    "1.0",
			wantErr: false,
		},
		{
			name:    "valid_v1.1",
			input:   "@c4m 1.1\n",
			want:    "1.1",
			wantErr: false,
		},
		{
			name:    "invalid_header",
			input:   "not a c4m file\n",
			want:    "",
			wantErr: true,
		},
		{
			name:    "missing_version",
			input:   "@c4m \n",
			want:    "",
			wantErr: true,
		},
		{
			name:    "unsupported_version",
			input:   "@c4m 2.0\n",
			want:    "",
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewDecoder(strings.NewReader(tt.input))
			err := parser.parseHeader()
			
			if (err != nil) != tt.wantErr {
				t.Errorf("parseHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && parser.Version() != tt.want {
				t.Errorf("Version() = %v, want %v", parser.Version(), tt.want)
			}
		})
	}
}

func TestDecoder_parseEntry(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Entry
		wantErr bool
	}{
		{
			name:  "regular_file",
			input: "@c4m 1.0\n-rw-r--r-- 2023-01-01T12:00:00Z 1024 README.md c43zYcLni5LF9rR4Lg4B8h3Jp8SBwjcnyyeh4bc6gTPHndKuKdjUWx1kJPYhZxYt3zV6tQXpDs2shPsPYjgG81wZM1\n",
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
			input: "@c4m 1.0\ndrwxr-xr-x 2023-01-01T12:00:00Z 4096 src/\n",
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
			input: "@c4m 1.0\nlrwxrwxrwx 2023-01-01T12:00:00Z 0 link.txt -> target.txt\n",
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
			input: "@c4m 1.0\n-rw-r--r-- 2023-01-01T12:00:00Z 2048 \"my file.txt\"\n",
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
			input: "@c4m 1.0\n  -rw-r--r-- 2023-01-01T12:00:00Z 512 nested.txt\n",
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
			input:   "@c4m 1.0\ninvalid 2023-01-01T12:00:00Z 1024 file.txt\n",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid_timestamp",
			input:   "@c4m 1.0\n-rw-r--r-- not-a-timestamp 1024 file.txt\n",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid_size",
			input:   "@c4m 1.0\n-rw-r--r-- 2023-01-01T12:00:00Z not-a-size file.txt\n",
			want:    nil,
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewDecoder(strings.NewReader(tt.input))
			
			// Parse header first
			if err := parser.parseHeader(); err != nil {
				t.Fatalf("parseHeader() failed: %v", err)
			}
			
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
			name: "basic manifest",
			input: `@c4m 1.0
-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt`,
			check: func(t *testing.T, m *Manifest) {
				if m.Version != "1.0" {
					t.Errorf("Version = %q, want 1.0", m.Version)
				}
				if len(m.Entries) != 1 {
					t.Fatalf("Entries = %d, want 1", len(m.Entries))
				}
				if m.Entries[0].Name != "file.txt" {
					t.Errorf("Name = %q, want file.txt", m.Entries[0].Name)
				}
			},
		},
		{
			name: "manifest with base directive",
			input: `@c4m 1.0
@base c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN
-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt`,
			check: func(t *testing.T, m *Manifest) {
				if m.Base.IsNil() {
					t.Error("Base should not be nil")
				}
			},
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
		{
			name: "multiple entries",
			input: `@c4m 1.0
-rw-r--r-- 2024-01-01T00:00:00Z 100 file1.txt
-rw-r--r-- 2024-01-01T00:00:00Z 200 file2.txt
drwxr-xr-x 2024-01-01T00:00:00Z 0 dir/`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Entries) != 3 {
					t.Fatalf("Entries = %d, want 3", len(m.Entries))
				}
			},
		},
		{
			name: "symlink entry",
			input: `@c4m 1.0
lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> target`,
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
			name: "symlink with spaces in target",
			input: `@c4m 1.0
lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> "target with spaces"`,
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
			name: "symlink to absolute path",
			input: `@c4m 1.0
lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> /absolute/path/target`,
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
			name:    "invalid header",
			input:   `@c4 1.0
-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt`,
			wantErr: true,
		},
		{
			name:    "unsupported version",
			input:   `@c4m 2.0
-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
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

func TestDirectiveError(t *testing.T) {
	input := `@c4m 1.0
@base c416ujTTpmKJwJM1bS1NM7F42WNSKAeLMzKWNfUjH7pNJkLHQGN6MDAJfLCeTEmGfSaW6mPo7xWzFRKCUQrEXJxY5KNP`

	p := NewDecoder(strings.NewReader(input))
	_ = p.parseHeader()
	
	_, err := p.parseEntry()
	if err == nil {
		t.Error("Expected directiveError")
	}
	
	if _, ok := err.(*directiveError); !ok {
		t.Errorf("Expected directiveError, got %T", err)
	}
}

func TestDecoder_parseEntryEOF(t *testing.T) {
	input := `@c4m 1.0`
	
	p := NewDecoder(strings.NewReader(input))
	_ = p.parseHeader()
	
	_, err := p.parseEntry()
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestHandleDirective(t *testing.T) {
	tests := []struct {
		name      string
		directive string
		manifest  *Manifest
		wantErr   bool
		check     func(t *testing.T, m *Manifest)
	}{
		{
			name:      "base directive",
			directive: "@base c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN",
			manifest:  &Manifest{},
			check: func(t *testing.T, m *Manifest) {
				if m.Base.IsNil() {
					t.Error("Base should not be nil")
				}
			},
		},
		{
			name:      "base directive missing id",
			directive: "@base",
			manifest:  &Manifest{},
			wantErr:   true,
		},
		{
			name:      "layer directive",
			directive: "@layer",
			manifest:  &Manifest{},
			check: func(t *testing.T, m *Manifest) {
				if m.CurrentLayer == nil {
					t.Fatal("CurrentLayer should not be nil")
				}
				if m.CurrentLayer.Type != LayerTypeAdd {
					t.Errorf("Layer type = %v, want LayerTypeAdd", m.CurrentLayer.Type)
				}
			},
		},
		{
			name:      "remove directive",
			directive: "@remove",
			manifest:  &Manifest{},
			check: func(t *testing.T, m *Manifest) {
				if m.CurrentLayer == nil {
					t.Fatal("CurrentLayer should not be nil")
				}
				if m.CurrentLayer.Type != LayerTypeRemove {
					t.Errorf("Layer type = %v, want LayerTypeRemove", m.CurrentLayer.Type)
				}
			},
		},
		{
			name:      "by directive",
			directive: "@by user@example.com",
			manifest:  &Manifest{CurrentLayer: &Layer{}},
			check: func(t *testing.T, m *Manifest) {
				if m.CurrentLayer.By != "user@example.com" {
					t.Errorf("Layer by = %q, want user@example.com", m.CurrentLayer.By)
				}
			},
		},
		{
			name:      "time directive",
			directive: "@time 2024-01-01T00:00:00Z",
			manifest:  &Manifest{CurrentLayer: &Layer{}},
			check: func(t *testing.T, m *Manifest) {
				expected, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
				if !m.CurrentLayer.Time.Equal(expected) {
					t.Errorf("Layer time = %v, want %v", m.CurrentLayer.Time, expected)
				}
			},
		},
		{
			name:      "note directive",
			directive: "@note This is a note",
			manifest:  &Manifest{CurrentLayer: &Layer{}},
			check: func(t *testing.T, m *Manifest) {
				if m.CurrentLayer.Note != "This is a note" {
					t.Errorf("Layer note = %q, want 'This is a note'", m.CurrentLayer.Note)
				}
			},
		},
		{
			name:      "data directive on manifest",
			directive: "@data c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN",
			manifest:  &Manifest{},
			check: func(t *testing.T, m *Manifest) {
				if m.Data.IsNil() {
					t.Error("Data should not be nil")
				}
			},
		},
		{
			name:      "data directive on layer",
			directive: "@data c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN",
			manifest:  &Manifest{CurrentLayer: &Layer{}},
			check: func(t *testing.T, m *Manifest) {
				if m.CurrentLayer.Data.IsNil() {
					t.Error("Layer Data should not be nil")
				}
			},
		},
		{
			name:      "expand directive returns error",
			directive: "@expand c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN",
			manifest:  &Manifest{},
			wantErr:   true,
		},
		{
			name:      "expand directive without id also returns error",
			directive: "@expand",
			manifest:  &Manifest{},
			wantErr:   true,
		},
		{
			name:      "unknown directive",
			directive: "@unknown",
			manifest:  &Manifest{},
			wantErr:   false,
		},
		{
			name:      "empty directive",
			directive: "@",
			manifest:  &Manifest{},
			wantErr:   false,
		},
		{
			name:      "invalid base c4 id",
			directive: "@base invalid",
			manifest:  &Manifest{},
			wantErr:   true,
		},
		{
			name:      "invalid data c4 id",
			directive: "@data not_a_c4_id",
			manifest:  &Manifest{},
			wantErr:   true,
		},
		{
			name:      "invalid time format",
			directive: "@time not_a_time",
			manifest:  &Manifest{CurrentLayer: &Layer{}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewDecoder(strings.NewReader(""))
			err := p.handleDirective(tt.manifest, tt.directive)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleDirective() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, tt.manifest)
			}
		})
	}
}

func TestDirectiveErrorType(t *testing.T) {
	err := &directiveError{directive: "@test"}
	expected := "directive: @test"
	if err.Error() != expected {
		t.Errorf("directiveError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestParserNew(t *testing.T) {
	// Test parsing a complete manifest
	content := `@c4m 1.0
@base c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
drwxr-xr-x 2025-09-19T12:00:00Z 200 dir/
  -rw-r--r-- 2025-09-19T12:00:00Z 200 file.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
`

	parser := NewDecoder(strings.NewReader(content))

	// Use Decode which handles header internally
	manifest, err := parser.Decode()
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	if manifest.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", manifest.Version)
	}

	if manifest.Base.IsNil() {
		t.Error("Expected @base to be parsed")
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

func TestParseDataBlock(t *testing.T) {
	// Create a test ID list to embed
	idList := NewIDList()
	id1 := c4.Identify(strings.NewReader("content1\n"))
	id2 := c4.Identify(strings.NewReader("content2\n"))
	idList.Add(id1)
	idList.Add(id2)

	// Create the data block
	block := CreateDataBlockFromIDList(idList)

	// Build manifest with embedded @data block
	content := fmt.Sprintf(`@c4m 1.0
-rw-r--r-- 2024-01-15T10:30:00Z 100 files[0001-0002].txt %s
@data %s
%s
%s
`, block.ID.String(), block.ID.String(), id1.String(), id2.String())

	parser := NewDecoder(strings.NewReader(content))
	manifest, err := parser.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	// Verify the data block was parsed
	if len(manifest.DataBlocks) != 1 {
		t.Fatalf("Expected 1 data block, got %d", len(manifest.DataBlocks))
	}

	// Verify we can retrieve it
	retrieved := manifest.GetDataBlock(block.ID)
	if retrieved == nil {
		t.Fatal("GetDataBlock() returned nil")
	}

	// Verify the content matches
	if !retrieved.IsIDList {
		t.Error("Expected data block to be an ID list")
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

func TestHandleDataBlockConsecutive(t *testing.T) {
	// Test consecutive @data blocks using valid ID list blocks
	// Create first ID list
	list1 := NewIDList()
	list1.Add(c4.Identify(strings.NewReader("file1")))
	list1.Add(c4.Identify(strings.NewReader("file2")))
	block1 := CreateDataBlockFromIDList(list1)

	// Create second ID list
	list2 := NewIDList()
	list2.Add(c4.Identify(strings.NewReader("file3")))
	list2.Add(c4.Identify(strings.NewReader("file4")))
	block2 := CreateDataBlockFromIDList(list2)

	input := fmt.Sprintf("@c4m 1.0\n%s%s", FormatDataBlock(block1), FormatDataBlock(block2))

	parser := NewDecoder(strings.NewReader(input))
	manifest, err := parser.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if len(manifest.DataBlocks) != 2 {
		t.Fatalf("Expected 2 data blocks, got %d", len(manifest.DataBlocks))
	}
}

