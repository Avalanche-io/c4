//go:build go1.18
// +build go1.18

package c4m

import (
	"bytes"
	"sort"
	"testing"
)

// FuzzDecoder feeds random input to the decoder — must not panic.
func FuzzDecoder(f *testing.F) {
	f.Add([]byte("-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt\n"))
	f.Add([]byte(""))
	f.Add([]byte("---------- - - \"quoted name\"\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic regardless of input
		Unmarshal(data)
	})
}

// FuzzRoundTrip builds valid manifests, encodes, decodes, re-encodes.
// The two encodings must be byte-identical.
func FuzzRoundTrip(f *testing.F) {
	f.Add("hello.txt", int64(100))
	f.Add("file with spaces.txt", int64(0))
	f.Add("back\\slash.txt", int64(42))

	f.Fuzz(func(t *testing.T, name string, size int64) {
		if len(name) == 0 || len(name) > 255 {
			return
		}
		// Skip names with control characters, null bytes, or path separators
		for _, c := range name {
			if c < 0x20 || c == 0 || c == '/' {
				return
			}
		}
		// Skip names starting with @ (directives)
		if name[0] == '@' {
			return
		}

		m := NewManifest()
		m.AddEntry(&Entry{
			Name: name,
			Mode: 0644,
			Size: size,
		})

		enc1, err := Marshal(m)
		if err != nil {
			return // Some names may not be encodable
		}

		decoded, err := Unmarshal(enc1)
		if err != nil {
			t.Fatalf("failed to decode valid manifest: %v\nEncoded:\n%s", err, enc1)
		}

		enc2, err := Marshal(decoded)
		if err != nil {
			t.Fatalf("failed to re-encode: %v", err)
		}

		if !bytes.Equal(enc1, enc2) {
			t.Errorf("round-trip mismatch:\nFirst:  %s\nSecond: %s", enc1, enc2)
		}
	})
}

// FuzzValidator feeds random input to the validator — must not panic.
func FuzzValidator(f *testing.F) {
	f.Add("-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt\n")
	f.Add("")

	f.Fuzz(func(t *testing.T, data string) {
		v := NewValidator(false)
		v.ValidateManifest(bytes.NewReader([]byte(data)))
	})
}

// FuzzNaturalSort feeds random strings to NaturalLess — must not panic
// and must maintain transitivity.
func FuzzNaturalSort(f *testing.F) {
	f.Add("file1.txt", "file2.txt", "file10.txt")
	f.Add("abc", "123", "xyz")
	f.Add("", "a", "")

	f.Fuzz(func(t *testing.T, a, b, c string) {
		// Must not panic
		NaturalLess(a, b)
		NaturalLess(b, c)
		NaturalLess(a, c)

		// Test transitivity: if a < b and b < c then a < c
		if NaturalLess(a, b) && NaturalLess(b, c) {
			if !NaturalLess(a, c) {
				t.Errorf("transitivity violated: %q < %q < %q but not %q < %q", a, b, c, a, c)
			}
		}

		// Test anti-symmetry: if a < b then !(b < a)
		if NaturalLess(a, b) && NaturalLess(b, a) {
			t.Errorf("anti-symmetry violated: %q < %q and %q < %q", a, b, b, a)
		}

		// Must be sortable without panic
		s := []string{a, b, c}
		sort.Slice(s, func(i, j int) bool {
			return NaturalLess(s[i], s[j])
		})
	})
}
