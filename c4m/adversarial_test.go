package c4m

import (
	"strings"
	"testing"
)

// TestAdversarial_MalformedHeaders tests decoder robustness against invalid headers.
func TestAdversarial_MalformedHeaders(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"no header", "-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt\n"},
		{"wrong prefix", "@c5m 1.0\n"},
		{"no version", "@c4m \n"},
		{"unsupported version", "@c4m 2.0\n"},
		{"partial header", "@c4m"},
		{"header with garbage", "@c4m 1.0 extra stuff\n"},
		{"null bytes in header", "@c4m\x001.0\n"},
		{"just newlines", "\n\n\n"},
		{"binary garbage", "\x00\xff\xfe\xfd\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDecoder(strings.NewReader(tc.input))
			// Must not panic
			d.Decode()
		})
	}
}

// TestAdversarial_CorruptEntries tests decoder robustness against malformed entry lines.
func TestAdversarial_CorruptEntries(t *testing.T) {
	header := "@c4m 1.0\n"
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
			input := header + tc.entry + "\n"
			d := NewDecoder(strings.NewReader(input))
			// Must not panic — errors are acceptable
			d.Decode()
		})
	}
}

// TestAdversarial_DeepNesting tests that deeply indented entries don't cause problems.
func TestAdversarial_DeepNesting(t *testing.T) {
	var b strings.Builder
	b.WriteString("@c4m 1.0\n")
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
	b.WriteString("@c4m 1.0\n")
	for i := 0; i < 10000; i++ {
		b.WriteString("-rw-r--r-- 2024-01-01T00:00:00Z 100 file_")
		b.WriteString(strings.Repeat("x", 10))
		b.WriteString("\n")
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
	header := "@c4m 1.0\n"
	longName := strings.Repeat("a", 4096)
	input := header + "-rw-r--r-- 2024-01-01T00:00:00Z 100 " + longName + "\n"
	d := NewDecoder(strings.NewReader(input))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("long name decode failed: %v", err)
	}
	if len(m.Entries) != 1 || m.Entries[0].Name != longName {
		t.Errorf("long name not preserved")
	}
}

// TestAdversarial_QuotedNameEdgeCases tests tricky quoting scenarios.
func TestAdversarial_QuotedNameEdgeCases(t *testing.T) {
	header := "@c4m 1.0\n"
	cases := []struct {
		name     string
		entry    string
		wantName string
	}{
		{
			"empty quoted name",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 ""`,
			"",
		},
		{
			"quoted with escaped backslash",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 "back\\slash"`,
			`back\slash`,
		},
		{
			"quoted with escaped quote",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 "has\"quote"`,
			`has"quote`,
		},
		{
			"quoted with escaped newline",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 "has\nnewline"`,
			"has\nnewline",
		},
		{
			"quoted with spaces",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 "my file name.txt"`,
			"my file name.txt",
		},
		{
			"quoted with arrow in name",
			`-rw-r--r-- 2024-01-01T00:00:00Z 100 "file -> not a link"`,
			"file -> not a link",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := header + tc.entry + "\n"
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

// TestAdversarial_DirectiveBombardment tests many directives in sequence.
func TestAdversarial_DirectiveBombardment(t *testing.T) {
	var b strings.Builder
	b.WriteString("@c4m 1.0\n")
	b.WriteString("@layer\n")
	b.WriteString("@by test user\n")
	b.WriteString("@note some note\n")
	b.WriteString("@time 2024-01-01T00:00:00Z\n")
	b.WriteString("-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt\n")
	b.WriteString("@end\n")
	b.WriteString("@layer\n")
	b.WriteString("@end\n")
	b.WriteString("@remove\n")
	b.WriteString("@end\n")

	d := NewDecoder(strings.NewReader(b.String()))
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("directive bombardment decode failed: %v", err)
	}
	if len(m.Layers) != 3 {
		t.Errorf("expected 3 layers, got %d", len(m.Layers))
	}
}

// TestAdversarial_RepeatedUnmarshal tests that Unmarshal doesn't leak state between calls.
func TestAdversarial_RepeatedUnmarshal(t *testing.T) {
	input := []byte("@c4m 1.0\n-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt\n")
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
	header := "@c4m 1.0\n"
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
			input := header + tc.entry + "\n"
			d := NewDecoder(strings.NewReader(input))
			// Must not panic — errors are acceptable for invalid combos
			d.Decode()
		})
	}
}
