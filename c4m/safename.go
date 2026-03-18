package c4m

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SafeName encodes a raw filesystem name using the Universal Filename
// Encoding (three-tier system). The result contains only printable UTF-8
// characters, with Tier 2 backslash escapes for common control chars and
// Tier 3 braille encoding for all other non-printable bytes.
//
// See design/filename-encoding.md for the full specification.
func SafeName(raw string) string {
	// Fast path: check if encoding is needed.
	safe := true
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[i:])
		if r == utf8.RuneError && size <= 1 {
			safe = false
			break
		}
		if r == '¤' || r == '\\' || !unicode.IsPrint(r) {
			safe = false
			break
		}
		i += size
	}
	if safe {
		return raw
	}

	var b strings.Builder
	b.Grow(len(raw))
	var pending []byte // Tier 3 accumulator for range encoding

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		b.WriteRune('¤')
		for _, c := range pending {
			b.WriteRune(rune(0x2800 + int(c)))
		}
		b.WriteRune('¤')
		pending = pending[:0]
	}

	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[i:])

		// Tier 1: printable UTF-8, not ¤, not backslash.
		if (r != utf8.RuneError || size > 1) && unicode.IsPrint(r) && r != '¤' && r != '\\' {
			flushPending()
			b.WriteString(raw[i : i+size])
			i += size
			continue
		}

		// Tier 2: backslash escapes for specific characters.
		if esc := tier2Escape(r); esc != 0 && (r != utf8.RuneError || size > 1) {
			flushPending()
			b.WriteByte('\\')
			b.WriteByte(esc)
			i += size
			continue
		}

		// Tier 3: accumulate bytes for range encoding.
		if r == utf8.RuneError && size <= 1 {
			pending = append(pending, raw[i])
			i++
		} else {
			for j := 0; j < size; j++ {
				pending = append(pending, raw[i+j])
			}
			i += size
		}
	}
	flushPending()

	return b.String()
}

// UnsafeName reverses SafeName: decodes Tier 2 backslash escapes and
// Tier 3 braille patterns back to raw bytes.
func UnsafeName(encoded string) string {
	if !strings.ContainsAny(encoded, "¤\\") {
		return encoded
	}

	var b strings.Builder
	b.Grow(len(encoded))

	for i := 0; i < len(encoded); {
		r, size := utf8.DecodeRuneInString(encoded[i:])

		// Tier 2: backslash escape.
		if r == '\\' {
			if i+size < len(encoded) {
				next := encoded[i+size]
				if val, ok := tier2Unescape(next); ok {
					b.WriteByte(val)
					i += size + 1
					continue
				}
			}
			// Lone backslash or unknown escape — pass through.
			b.WriteByte('\\')
			i += size
			continue
		}

		// Tier 3: ¤…¤ braille range.
		if r == '¤' {
			j := i + size
			decoded := false
			for j < len(encoded) {
				br, bsize := utf8.DecodeRuneInString(encoded[j:])
				if br == '¤' {
					if decoded {
						i = j + bsize
					} else {
						b.WriteRune('¤')
						i += size
					}
					goto next
				}
				if br >= 0x2800 && br <= 0x28FF {
					b.WriteByte(byte(br - 0x2800))
					decoded = true
					j += bsize
					continue
				}
				break
			}
			b.WriteRune('¤')
			i += size
			continue
		}

		// Tier 1: passthrough.
		b.WriteString(encoded[i : i+size])
		i += size
		continue

	next:
	}

	return b.String()
}

// tier2Escape returns the escape character for a Tier 2 byte, or 0 if
// the rune is not a Tier 2 character.
func tier2Escape(r rune) byte {
	switch r {
	case 0x00:
		return '0'
	case '\t':
		return 't'
	case '\n':
		return 'n'
	case '\r':
		return 'r'
	case '\\':
		return '\\'
	}
	return 0
}

// tier2Unescape returns the byte value for a Tier 2 escape character.
func tier2Unescape(c byte) (byte, bool) {
	switch c {
	case '0':
		return 0x00, true
	case 't':
		return '\t', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case '\\':
		return '\\', true
	}
	return 0, false
}
