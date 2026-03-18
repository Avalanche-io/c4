package c4m

import (
	"testing"
)

func TestSafeNameTier1Passthrough(t *testing.T) {
	tests := []string{
		"hello.txt",
		"café",
		"日本語.txt",
		"path/to/file",
		"normal-file_name.2024",
		"",
		// Spaces and quotes pass through Tier 1.
		"hello world",
		`he said "hi"`,
	}
	for _, raw := range tests {
		got := SafeName(raw)
		if got != raw {
			t.Errorf("SafeName(%q) = %q, want passthrough", raw, got)
		}
	}
}

func TestSafeNameTier2Escapes(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"a\tb", `a\tb`},
		{"a\nb", `a\nb`},
		{"a\rb", `a\rb`},
		{"a\x00b", `a\0b`},
		{`a\b`, `a\\b`},
		// Multiple Tier 2 chars.
		{"\t\n\r", `\t\n\r`},
		// Backslash at end.
		{`test\`, `test\\`},
	}
	for _, tt := range tests {
		got := SafeName(tt.raw)
		if got != tt.want {
			t.Errorf("SafeName(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestSafeNameTier3Single(t *testing.T) {
	// Single non-printable byte 0x01.
	raw := "a\x01b"
	got := SafeName(raw)
	want := "a\u00a4\u2801\u00a4b" // a¤⠁¤b
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestSafeNameTier3Range(t *testing.T) {
	// Consecutive non-printable bytes share delimiters.
	raw := "a\x01\x02\x03b"
	got := SafeName(raw)
	want := "a\u00a4\u2801\u2802\u2803\u00a4b" // a¤⠁⠂⠃¤b
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestSafeNameCurrencySymbol(t *testing.T) {
	// ¤ (U+00A4) encoded via its UTF-8 bytes (0xC2, 0xA4) in Tier 3.
	raw := "price\u00a4tag"
	got := SafeName(raw)
	want := "price\u00a4\u28c2\u28a4\u00a4tag"
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestSafeNameBackslashLiteral(t *testing.T) {
	raw := "path\\to\\file"
	got := SafeName(raw)
	want := `path\\to\\file`
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestSafeNameNonPrintableUnicode(t *testing.T) {
	// U+200B (zero-width space) — UTF-8: E2 80 8B
	raw := "a\u200Bb"
	got := SafeName(raw)
	want := "a\u00a4\u28e2\u2880\u288b\u00a4b"
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestSafeNameTier2BreaksTier3Range(t *testing.T) {
	// \x01 is Tier 3, \n is Tier 2 — Tier 2 flushes the Tier 3 range.
	raw := "a\x01\nb"
	got := SafeName(raw)
	want := "a\u00a4\u2801\u00a4\\nb" // a¤⠁¤\nb
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestSafeNameInvalidUTF8(t *testing.T) {
	raw := "a\xff\xfeb"
	got := SafeName(raw)
	want := "a\u00a4\u28ff\u28fe\u00a4b"
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestSafeNameMixedTiers(t *testing.T) {
	raw := "hello\t\x01\x02world"
	got := SafeName(raw)
	want := "hello\\t\u00a4\u2801\u2802\u00a4world"
	if got != want {
		t.Errorf("SafeName(%q) = %q, want %q", raw, got, want)
	}
}

func TestUnsafeNamePassthrough(t *testing.T) {
	tests := []string{
		"hello.txt",
		"café",
		"normal-file",
		"",
		"hello world",
	}
	for _, enc := range tests {
		got := UnsafeName(enc)
		if got != enc {
			t.Errorf("UnsafeName(%q) = %q, want passthrough", enc, got)
		}
	}
}

func TestUnsafeNameTier2(t *testing.T) {
	tests := []struct {
		enc  string
		want string
	}{
		{`a\tb`, "a\tb"},
		{`a\nb`, "a\nb"},
		{`a\rb`, "a\rb"},
		{`a\0b`, "a\x00b"},
		{`a\\b`, "a\\b"},
	}
	for _, tt := range tests {
		got := UnsafeName(tt.enc)
		if got != tt.want {
			t.Errorf("UnsafeName(%q) = %q, want %q", tt.enc, got, tt.want)
		}
	}
}

func TestUnsafeNameTier3Range(t *testing.T) {
	enc := "a\u00a4\u2801\u2802\u2803\u00a4b"
	got := UnsafeName(enc)
	want := "a\x01\x02\x03b"
	if got != want {
		t.Errorf("UnsafeName(%q) = %q, want %q", enc, got, want)
	}
}

func TestUnsafeNameCurrencySymbol(t *testing.T) {
	enc := "price\u00a4\u28c2\u28a4\u00a4tag"
	got := UnsafeName(enc)
	want := "price\u00a4tag"
	if got != want {
		t.Errorf("UnsafeName(%q) = %q, want %q", enc, got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []string{
		"hello.txt",
		"café",
		"a\tb",
		"a\nb",
		"a\rb",
		"a\x00b",
		"a\\b",
		"price\u00a4tag",
		"\u00a4\u00a4\u00a4",
		"a\x01\x02\x03b",
		"a\xff\xfeb",
		"\t\n\r\x00\\",
		"a\x01\nb",
		"\x00\x01\x02\x03\x04\x05",
		"a\u200Bb",
		"hello\t\x01\x02world",
		"hello world",
		`path\to\file`,
		`he said "hi"`,
		// All 256 byte values.
		func() string {
			var buf [256]byte
			for i := range buf {
				buf[i] = byte(i)
			}
			return string(buf[:])
		}(),
		// Real-world problem case: filename with CR.
		"file\rwith\rCR",
	}
	for _, raw := range tests {
		enc := SafeName(raw)
		dec := UnsafeName(enc)
		if dec != raw {
			t.Errorf("round-trip failed: raw=%q → enc=%q → dec=%q", raw, enc, dec)
		}
	}
}

func TestUnsafeNameEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		enc  string
		want string
	}{
		{"trailing backslash", `test\`, `test\`},
		{"unknown escape", `test\x`, `test\x`},
		{"lone currency", "test\u00a4", "test\u00a4"},
		{"empty delimiters", "test\u00a4\u00a4end", "test\u00a4\u00a4end"},
		{"non-braille content", "test\u00a4abc\u00a4end", "test\u00a4abc\u00a4end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnsafeName(tt.enc)
			if got != tt.want {
				t.Errorf("UnsafeName(%q) = %q, want %q", tt.enc, got, tt.want)
			}
		})
	}
}
