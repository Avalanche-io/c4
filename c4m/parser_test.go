package c4m

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseHeader(t *testing.T) {
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
			parser := NewParser(strings.NewReader(tt.input))
			err := parser.ParseHeader()
			
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && parser.Version() != tt.want {
				t.Errorf("Version() = %v, want %v", parser.Version(), tt.want)
			}
		})
	}
}

func TestParseEntry(t *testing.T) {
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
			parser := NewParser(strings.NewReader(tt.input))
			
			// Parse header first
			if err := parser.ParseHeader(); err != nil {
				t.Fatalf("ParseHeader() failed: %v", err)
			}
			
			// Parse entry
			got, err := parser.ParseEntry()
			
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEntry() error = %v, wantErr %v", err, tt.wantErr)
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

func TestParseFields(t *testing.T) {
	parser := NewParser(strings.NewReader(""))
	
	tests := []struct {
		name   string
		input  string
		want   []string
	}{
		{
			name:  "simple_fields",
			input: "field1 field2 field3",
			want:  []string{"field1", "field2", "field3"},
		},
		{
			name:  "quoted_field",
			input: `field1 "field with spaces" field3`,
			want:  []string{"field1", "field with spaces", "field3"},
		},
		{
			name:  "escaped_quotes",
			input: `"field with \"quotes\""`,
			want:  []string{`field with "quotes"`},
		},
		{
			name:  "escaped_newline",
			input: `"field with\nnewline"`,
			want:  []string{"field with\nnewline"},
		},
		{
			name:  "multiple_spaces",
			input: "field1    field2     field3",
			want:  []string{"field1", "field2", "field3"},
		},
		{
			name:  "symlink_arrow",
			input: "link -> target",
			want:  []string{"link", "->", "target"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.parseFields(tt.input)
			
			if len(got) != len(tt.want) {
				t.Fatalf("got %d fields, want %d", len(got), len(tt.want))
			}
			
			for i, field := range got {
				if field != tt.want[i] {
					t.Errorf("field %d: got %q, want %q", i, field, tt.want[i])
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

func TestParseAll(t *testing.T) {
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
			p := NewParser(strings.NewReader(tt.input))
			m, err := p.ParseAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAll() error = %v, wantErr %v", err, tt.wantErr)
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

	p := NewParser(strings.NewReader(input))
	_ = p.ParseHeader()
	
	_, err := p.ParseEntry()
	if err == nil {
		t.Error("Expected DirectiveError")
	}
	
	if _, ok := err.(*DirectiveError); !ok {
		t.Errorf("Expected DirectiveError, got %T", err)
	}
}

func TestParseEntryEOF(t *testing.T) {
	input := `@c4m 1.0`
	
	p := NewParser(strings.NewReader(input))
	_ = p.ParseHeader()
	
	_, err := p.ParseEntry()
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
			name:      "expand directive",
			directive: "@expand c41HX1X4uedbqHB72FCDXFnifrN1PTWfFZfV2Hh6y3RE9dUy5wJrgzmf9tWnyR9B29AvoJsKNd7RhFbxbumvBtSjtN",
			manifest:  &Manifest{},
			wantErr:   false,
		},
		{
			name:      "expand directive missing id",
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
			p := NewParser(strings.NewReader(""))
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

func TestStrictParser(t *testing.T) {
	p := NewStrictParser(strings.NewReader("test"))
	if !p.strict {
		t.Error("NewStrictParser should set strict mode")
	}
}

func TestDirectiveErrorType(t *testing.T) {
	err := &DirectiveError{Directive: "@test"}
	expected := "directive: @test"
	if err.Error() != expected {
		t.Errorf("DirectiveError.Error() = %q, want %q", err.Error(), expected)
	}
}