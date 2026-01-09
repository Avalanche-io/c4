package c4m

import (
	"bytes"
	"io"
)

// Marshal returns the canonical C4M encoding of m.
func Marshal(m *Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalPretty returns the pretty-printed C4M encoding of m.
// Pretty format includes aligned columns, formatted sizes with commas,
// and timestamps in local time.
func MarshalPretty(m *Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf).SetPretty(true)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal parses the C4M-encoded data and returns a Manifest.
func Unmarshal(data []byte) (*Manifest, error) {
	dec := NewDecoder(bytes.NewReader(data))
	return dec.Decode()
}

// Format parses and re-formats src in canonical C4M style.
// It returns the formatted result or an error if src is not valid C4M.
func Format(src []byte) ([]byte, error) {
	m, err := Unmarshal(src)
	if err != nil {
		return nil, err
	}
	return Marshal(m)
}

// FormatPretty parses and re-formats src in pretty-printed C4M style.
// It returns the formatted result or an error if src is not valid C4M.
func FormatPretty(src []byte) ([]byte, error) {
	m, err := Unmarshal(src)
	if err != nil {
		return nil, err
	}
	return MarshalPretty(m)
}

// GenerateFromReader parses a manifest from r.
// This is a convenience function equivalent to NewDecoder(r).Decode().
func GenerateFromReader(r io.Reader) (*Manifest, error) {
	return NewDecoder(r).Decode()
}
