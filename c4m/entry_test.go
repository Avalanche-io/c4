package c4m

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestEntryIsDir(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{"regular file", 0644, false},
		{"directory", os.ModeDir | 0755, true},
		{"symlink", os.ModeSymlink | 0777, false},
		{"device", os.ModeDevice | 0666, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Mode: tt.mode}
			if got := e.IsDir(); got != tt.want {
				t.Errorf("IsDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryIsSymlink(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{"regular file", 0644, false},
		{"directory", os.ModeDir | 0755, false},
		{"symlink", os.ModeSymlink | 0777, true},
		{"symlink to dir", os.ModeSymlink | os.ModeDir | 0777, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Mode: tt.mode}
			if got := e.IsSymlink(); got != tt.want {
				t.Errorf("IsSymlink() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryBaseName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"simple file", "file.txt", "file.txt"},
		{"path with directory", "dir/file.txt", "file.txt"},
		{"nested path", "a/b/c/file.txt", "file.txt"},
		{"directory", "mydir/", "mydir"},
		{"root", "/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Name: tt.filename}
			if got := e.BaseName(); got != tt.want {
				t.Errorf("BaseName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryString(t *testing.T) {
	e := &Entry{
		Mode:      0644,
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Size:      1234,
		Name:      "test.txt",
		C4ID:      c4.ID{},
	}

	// String() should call Format(0, false)
	got := e.String()
	want := e.Format(0, false)
	if got != want {
		t.Errorf("String() = %v, want %v", got, want)
	}
}

func TestEntryFormat(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	testID, _ := c4.Parse("c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")

	tests := []struct {
		name          string
		entry         *Entry
		indentWidth   int
		displayFormat bool
		want          string
	}{
		{
			name: "basic file no indent",
			entry: &Entry{
				Mode:      0644,
				Timestamp: testTime,
				Size:      1234,
				Name:      "test.txt",
				C4ID:      testID,
				Depth:     0,
			},
			indentWidth:   2,
			displayFormat: false,
			want:          "-rw-r--r-- 2024-01-15T10:30:00Z 1234 test.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "file with indent",
			entry: &Entry{
				Mode:      0644,
				Timestamp: testTime,
				Size:      1234,
				Name:      "test.txt",
				C4ID:      testID,
				Depth:     1,
			},
			indentWidth:   2,
			displayFormat: false,
			want:          "  -rw-r--r-- 2024-01-15T10:30:00Z 1234 test.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "directory",
			entry: &Entry{
				Mode:      os.ModeDir | 0755,
				Timestamp: testTime,
				Size:      4096,
				Name:      "mydir/",
				Depth:     0,
			},
			indentWidth:   2,
			displayFormat: false,
			want:          "drwxr-xr-x 2024-01-15T10:30:00Z 4096 mydir/ -",
		},
		{
			name: "symlink",
			entry: &Entry{
				Mode:      os.ModeSymlink | 0777,
				Timestamp: testTime,
				Size:      0,
				Name:      "link",
				Target:    "target.txt",
				Depth:     0,
			},
			indentWidth:   2,
			displayFormat: false,
			want:          "lrwxrwxrwx 2024-01-15T10:30:00Z 0 link -> target.txt -",
		},
		{
			name: "file with spaces escaped",
			entry: &Entry{
				Mode:      0644,
				Timestamp: testTime,
				Size:      100,
				Name:      "my file.txt",
				Depth:     0,
			},
			indentWidth:   2,
			displayFormat: false,
			want:          `-rw-r--r-- 2024-01-15T10:30:00Z 100 my\ file.txt -`,
		},
		{
			name: "display format with commas",
			entry: &Entry{
				Mode:      0644,
				Timestamp: testTime,
				Size:      1234567,
				Name:      "big.txt",
				Depth:     0,
			},
			indentWidth:   2,
			displayFormat: true,
			want:          "-rw-r--r-- 2024-01-15T10:30:00Z 1,234,567 big.txt -",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.Format(tt.indentWidth, tt.displayFormat)
			if got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEntryCanonical(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	testID, _ := c4.Parse("c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB")

	tests := []struct {
		name  string
		entry *Entry
		want  string
	}{
		{
			name: "file with C4 ID",
			entry: &Entry{
				Mode:      0644,
				Timestamp: testTime,
				Size:      1234,
				Name:      "test.txt",
				C4ID:      testID,
				Depth:     1, // Should be ignored in canonical
			},
			want: "-rw-r--r-- 2024-01-15T10:30:00Z 1234 test.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "directory",
			entry: &Entry{
				Mode:      os.ModeDir | 0755,
				Timestamp: testTime,
				Size:      4096,
				Name:      "mydir/",
				C4ID:      testID,
				Depth:     2,
			},
			want: "drwxr-xr-x 2024-01-15T10:30:00Z 4096 mydir/ c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "symlink with target",
			entry: &Entry{
				Mode:      os.ModeSymlink | 0777,
				Timestamp: testTime,
				Size:      0,
				Name:      "link",
				Target:    "target.txt",
			},
			want: "lrwxrwxrwx 2024-01-15T10:30:00Z 0 link -> target.txt -",
		},
		{
			name: "file with spaces (escaped)",
			entry: &Entry{
				Mode:      0644,
				Timestamp: testTime,
				Size:      100,
				Name:      "my file.txt",
			},
			want: `-rw-r--r-- 2024-01-15T10:30:00Z 100 my\ file.txt -`,
		},
		{
			name: "null timestamp renders as dash",
			entry: &Entry{
				Mode:      0644,
				Timestamp: NullTimestamp(),
				Size:      1234,
				Name:      "test.txt",
				C4ID:      testID,
			},
			want: "-rw-r--r-- - 1234 test.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "null mode renders as dash",
			entry: &Entry{
				Mode:      0, // Null mode (not dir or symlink)
				Timestamp: testTime,
				Size:      100,
				Name:      "unknown.bin",
				C4ID:      testID,
			},
			want: "- 2024-01-15T10:30:00Z 100 unknown.bin c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "null size renders as dash",
			entry: &Entry{
				Mode:      0644,
				Timestamp: testTime,
				Size:      -1, // Null size
				Name:      "partial.txt",
				C4ID:      testID,
			},
			want: "-rw-r--r-- 2024-01-15T10:30:00Z - partial.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
		{
			name: "all null values render as dashes",
			entry: &Entry{
				Mode:      0,
				Timestamp: NullTimestamp(),
				Size:      -1,
				Name:      "mystery.dat",
				C4ID:      testID,
			},
			want: "- - - mystery.dat c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.Canonical()
			if got != tt.want {
				t.Errorf("Canonical() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatMode(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want string
	}{
		{"regular file 644", 0644, "-rw-r--r--"},
		{"regular file 755", 0755, "-rwxr-xr-x"},
		{"directory 755", os.ModeDir | 0755, "drwxr-xr-x"},
		{"symlink 777", os.ModeSymlink | 0777, "lrwxrwxrwx"},
		{"named pipe", os.ModeNamedPipe | 0666, "prw-rw-rw-"},
		{"socket", os.ModeSocket | 0666, "srw-rw-rw-"},
		{"block device", os.ModeDevice | 0666, "brw-rw-rw-"},
		{"char device", os.ModeCharDevice | 0666, "crw-rw-rw-"},
		{"setuid", os.ModeSetuid | 0755, "-rwsr-xr-x"},
		{"setgid", os.ModeSetgid | 0755, "-rwxr-sr-x"},
		{"sticky", os.ModeSticky | 0755, "-rwxr-xr-t"},
		{"setuid no exec", os.ModeSetuid | 0644, "-rwSr--r--"},
		{"setgid no exec", os.ModeSetgid | 0644, "-rw-r-Sr--"},
		{"sticky no exec", os.ModeSticky | 0644, "-rw-r--r-T"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMode(tt.mode)
			if got != tt.want {
				t.Errorf("formatMode(%o) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name          string
		size          int64
		displayFormat bool
		want          string
	}{
		{"zero", 0, false, "0"},
		{"small", 123, false, "123"},
		{"thousand no display", 1234, false, "1234"},
		{"thousand with display", 1234, true, "1,234"},
		{"million with display", 1234567, true, "1,234,567"},
		{"billion with display", 1234567890, true, "1,234,567,890"},
		{"negative", -123, false, "-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSize(tt.size, tt.displayFormat)
			if got != tt.want {
				t.Errorf("formatSize(%d, %v) = %q, want %q", tt.size, tt.displayFormat, got, tt.want)
			}
		})
	}
}

func TestFormatName(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"simple name", "file.txt", "file.txt"},
		{"with spaces", "my file.txt", `my\ file.txt`},
		{"with quotes", `file"test".txt`, `file\"test\".txt`},
		// Backslash is SafeName-encoded as \\, no quoting needed.
		{"with backslash", `file\test.txt`, `file\\test.txt`},
		// Newline is SafeName-encoded as \n, no quoting needed.
		{"with newline", "file\ntest.txt", `file\ntest.txt`},
		{"leading space", " file.txt", `\ file.txt`},
		{"trailing space", "file.txt ", `file.txt\ `},
		{"multiple special", `my "special"\file.txt`, `my\ \"special\"\\file.txt`},
		{"no special chars", "file_test-123.txt", "file_test-123.txt"},
		{"dir simple", "mydir/", "mydir/"},
		{"dir with spaces", "my dir/", `my\ dir/`},
		{"dir with quotes", `dir"test"/`, `dir\"test\"/`},
		// Backslash SafeName-encoded, no quoting needed.
		{"dir with backslash", `dir\test/`, `dir\\test/`},
		// Newline SafeName-encoded, no quoting needed.
		{"dir with newline", "dir\ntest/", `dir\ntest/`},
		{"dir leading space", " dir/", `\ dir/`},
		{"brackets escaped", "file[1].txt", `file\[1\].txt`},
		{"brackets with backslash", `file\[1].txt`, `file\\\[1\].txt`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatName(tt.filename, false)
			if got != tt.want {
				t.Errorf("formatName(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestEntryIsDevice(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{"regular file", 0644, false},
		{"directory", os.ModeDir | 0755, false},
		{"block device", os.ModeDevice | 0666, true},
		{"char device", os.ModeCharDevice | 0666, true},
		{"symlink", os.ModeSymlink | 0777, false},
		{"named pipe", os.ModeNamedPipe | 0666, false},
		{"socket", os.ModeSocket | 0666, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Mode: tt.mode}
			if got := e.IsDevice(); got != tt.want {
				t.Errorf("IsDevice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryIsPipe(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{"regular file", 0644, false},
		{"directory", os.ModeDir | 0755, false},
		{"named pipe", os.ModeNamedPipe | 0666, true},
		{"socket", os.ModeSocket | 0666, false},
		{"device", os.ModeDevice | 0666, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Mode: tt.mode}
			if got := e.IsPipe(); got != tt.want {
				t.Errorf("IsPipe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryIsSocket(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{"regular file", 0644, false},
		{"directory", os.ModeDir | 0755, false},
		{"socket", os.ModeSocket | 0666, true},
		{"named pipe", os.ModeNamedPipe | 0666, false},
		{"device", os.ModeDevice | 0666, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Mode: tt.mode}
			if got := e.IsSocket(); got != tt.want {
				t.Errorf("IsSocket() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntryGetNullFields(t *testing.T) {
	tests := []struct {
		name   string
		entry  *Entry
		expect []string
	}{
		{
			name: "all fields set",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      100,
			},
			expect: nil,
		},
		{
			name: "null mode",
			entry: &Entry{
				Mode:      0,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      100,
			},
			expect: []string{"Mode"},
		},
		{
			name: "null timestamp",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Unix(0, 0),
				Size:      100,
			},
			expect: []string{"Timestamp"},
		},
		{
			name: "null size",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      -1,
			},
			expect: []string{"Size"},
		},
		{
			name: "all null",
			entry: &Entry{
				Mode:      0,
				Timestamp: time.Unix(0, 0),
				Size:      -1,
			},
			expect: []string{"Mode", "Timestamp", "Size"},
		},
		{
			name: "directory with mode 0 is ok",
			entry: &Entry{
				Mode:      os.ModeDir,
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Size:      0,
				Name:      "dir/",
			},
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.GetNullFields()
			if len(got) != len(tt.expect) {
				t.Errorf("GetNullFields() = %v, want %v", got, tt.expect)
				return
			}
			for i, field := range got {
				if field != tt.expect[i] {
					t.Errorf("GetNullFields()[%d] = %v, want %v", i, field, tt.expect[i])
				}
			}
		})
	}
}

func TestCanonicalNullTimestampConsistency(t *testing.T) {
	// Canonical() and Format() must agree on null timestamp rendering.
	// This was a bug: Format() rendered "-" but Canonical() rendered
	// "1970-01-01T00:00:00Z", causing C4 ID computation to use a
	// different value than the display representation.
	entry := &Entry{
		Mode:      0644,
		Timestamp: NullTimestamp(),
		Size:      100,
		Name:      "test.txt",
	}

	canonical := entry.Canonical()
	formatted := entry.Format(0, false)

	// Both should contain "-" for the timestamp, not epoch
	if strings.Contains(canonical, "1970") {
		t.Errorf("Canonical() rendered epoch for null timestamp: %s", canonical)
	}
	if strings.Contains(formatted, "1970") {
		t.Errorf("Format() rendered epoch for null timestamp: %s", formatted)
	}

	// Format(0, false) with depth=0 should match Canonical().
	// Verify timestamp field specifically. Split on spaces and check
	// field index 1 (timestamp).
	canonParts := strings.Fields(canonical)
	formatParts := strings.Fields(formatted)
	if len(canonParts) < 2 || len(formatParts) < 2 {
		t.Fatalf("unexpected field count: canonical=%d, format=%d",
			len(canonParts), len(formatParts))
	}
	if canonParts[1] != formatParts[1] {
		t.Errorf("timestamp field mismatch: Canonical()=%q, Format()=%q",
			canonParts[1], formatParts[1])
	}
}