package c4m

import (
	"fmt"
	"strings"
	"testing"
)

// TestAdversarial_MalformedInputs tests decoder robustness against invalid inputs.
func TestAdversarial_MalformedInputs(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty input", "", false},
		{"just newlines", "\n\n\n", false},
		{"binary garbage", "\x00\xff\xfe\xfd\n", true},
		{"directive line rejected", "@c4m 1.0\n", true},
		{"at-sign prefix rejected", "@anything\n", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDecoder(strings.NewReader(tc.input))
			// Must not panic
			_, err := d.Decode()
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestAdversarial_CorruptEntries tests decoder robustness against malformed entry lines.
func TestAdversarial_CorruptEntries(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"empty line", ""},
		{"just spaces", "          "},
		{"mode only", "-rw-r--r--"},
		{"mode and space", "-rw-r--r-- "},
		{"mode and timestamp only", "-rw-r--r-- 2024-01-01T00:00:00Z"},
		{"mode timestamp space", "-rw-r--r-- 2024-01-01T00:00:00Z "},
		{"mode timestamp size no name", "-rw-r--r-- 2024-01-01T00:00:00Z 100"},
		{"mode timestamp size space", "-rw-r--r-- 2024-01-01T00:00:00Z 100 "},
		{"truncated mode", "-rw-r-"},
		{"invalid mode char", "Xrw-r--r-- 2024-01-01T00:00:00Z 100 file.txt"},
		{"huge size", "-rw-r--r-- 2024-01-01T00:00:00Z 99999999999999999999 file.txt"},
		{"negative size", "-rw-r--r-- 2024-01-01T00:00:00Z -99 file.txt"},
		{"invalid timestamp", "-rw-r--r-- not-a-date 100 file.txt"},
		{"unterminated quote", `-rw-r--r-- 2024-01-01T00:00:00Z 100 "unterminated`},
		{"escape at end", `-rw-r--r-- 2024-01-01T00:00:00Z 100 "trailing\`},
		{"symlink no target", "-rw-r--r-- 2024-01-01T00:00:00Z 100 link -> "},
		{"just arrow", "-rw-r--r-- 2024-01-01T00:00:00Z 100 -> target"},
		{"invalid c4 id", "-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt c4INVALID"},
		{"null bytes in name", "-rw-r--r-- 2024-01-01T00:00:00Z 100 fi\x00le.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.entry + "\n"
			d := NewDecoder(strings.NewReader(input))
			// Must not panic — errors are acceptable
			d.Decode()
		})
	}
}

// TestAdversarial_DeepNesting tests that deeply indented entries don't cause problems.
func TestAdversarial_DeepNesting(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		indent := strings.Repeat("  ", i)
		b.WriteString(indent)
		b.WriteString("drwxr-xr-x 2024-01-01T00:00:00Z - dir/\n")
	}
	d := NewDecoder(strings.NewReader(b.String()))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("deep nesting decode failed: %v", err)
	}
	if len(m.Entries) != 100 {
		t.Errorf("expected 100 entries, got %d", len(m.Entries))
	}
}

// TestAdversarial_LargeManifest tests a manifest with many entries.
func TestAdversarial_LargeManifest(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&b, "-rw-r--r-- 2024-01-01T00:00:00Z 100 file_%05d.txt\n", i)
	}
	d := NewDecoder(strings.NewReader(b.String()))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("large manifest decode failed: %v", err)
	}
	if len(m.Entries) != 10000 {
		t.Errorf("expected 10000 entries, got %d", len(m.Entries))
	}
}

// TestAdversarial_LongNames tests entries with very long filenames.
func TestAdversarial_LongNames(t *testing.T) {
	longName := strings.Repeat("a", 4096)
	input := "-rw-r--r-- 2024-01-01T00:00:00Z 100 " + longName + "\n"
	d := NewDecoder(strings.NewReader(input))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("long name decode failed: %v", err)
	}
	if len(m.Entries) != 1 || m.Entries[0].Name != longName {
		t.Errorf("long name not preserved")
	}
}

// TestAdversarial_BackslashEscapedNameEdgeCases tests backslash-escape scenarios.
func TestAdversarial_BackslashEscapedNameEdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		entry    string
		wantName string
	}{
		{
			"escaped backslash via SafeName",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 back\\slash`,
			`back\slash`,
		},
		{
			"escaped quote",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 has\"quote`,
			`has"quote`,
		},
		{
			"escaped spaces",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 my\ file\ name.txt`,
			"my file name.txt",
		},
		{
			"escaped brackets",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 file\[v2\].txt`,
			"file[v2].txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.entry + "\n"
			d := NewDecoder(strings.NewReader(input))
			m, err := d.Decode()
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if len(m.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Entries))
			}
			if m.Entries[0].Name != tc.wantName {
				t.Errorf("name = %q, want %q", m.Entries[0].Name, tc.wantName)
			}
		})
	}
}

// TestAdversarial_RepeatedUnmarshal tests that Unmarshal doesn't leak state between calls.
func TestAdversarial_RepeatedUnmarshal(t *testing.T) {
	input := []byte("-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt\n")
	for i := 0; i < 100; i++ {
		m, err := Unmarshal(input)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if len(m.Entries) != 1 {
			t.Fatalf("iteration %d: expected 1 entry, got %d", i, len(m.Entries))
		}
	}
}

// TestAdversarial_NullFieldCombinations tests all combinations of null fields.
func TestAdversarial_NullFieldCombinations(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"all null", "- - - file.txt -"},
		{"null mode", "- 2024-01-01T00:00:00Z 100 file.txt"},
		{"null timestamp", "-rw-r--r-- - 100 file.txt"},
		{"null size", "-rw-r--r-- 2024-01-01T00:00:00Z - file.txt"},
		{"null c4id", "-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt -"},
		{"dashes everywhere", "---------- - - file.txt -"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.entry + "\n"
			d := NewDecoder(strings.NewReader(input))
			// Must not panic — errors are acceptable for invalid combos
			d.Decode()
		})
	}
}

// TestAdversarial_BackslashEscapedTarget tests backslash-escaped symlink targets.
func TestAdversarial_BackslashEscapedTarget(t *testing.T) {
	input := `lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> path\ with\ spaces` + "\n"
	d := NewDecoder(strings.NewReader(input))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	if m.Entries[0].Target != "path with spaces" {
		t.Errorf("Target = %q, want %q", m.Entries[0].Target, "path with spaces")
	}
}

// TestAdversarial_TargetWithNullC4ID tests symlink with null C4 ID marker.
func TestAdversarial_TargetWithNullC4ID(t *testing.T) {
	input := "lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> /some/target -\n"
	d := NewDecoder(strings.NewReader(input))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	e := m.Entries[0]
	if e.Name != "link" {
		t.Errorf("Name = %q, want %q", e.Name, "link")
	}
	if e.Target != "/some/target" {
		t.Errorf("Target = %q, want %q", e.Target, "/some/target")
	}
	if !e.C4ID.IsNil() {
		t.Errorf("C4ID should be nil for '-' marker, got %s", e.C4ID)
	}
}

// TestAdversarial_TargetWithEscapes tests a backslash-escaped symlink target
// with embedded quotes and backslashes.
func TestAdversarial_TargetWithEscapes(t *testing.T) {
	input := `lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> path\ with\ \"quotes\"\ and\ \\backslash` + "\n"
	d := NewDecoder(strings.NewReader(input))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	e := m.Entries[0]
	wantTarget := `path with "quotes" and \backslash`
	if e.Target != wantTarget {
		t.Errorf("Target = %q, want %q", e.Target, wantTarget)
	}
}
