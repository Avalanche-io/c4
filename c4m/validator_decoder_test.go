package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// The Validator reads through the decoder: on every shared vector file, in
// strict and lax mode, it refuses exactly the files the decoder refuses, at
// the decoder's line, and accepts every file the decoder reads (each is
// library-written, so in canonical order; strict mode also refuses a
// non-canonical indent unit). Before, its own parser disagreed
// with the decoder on 18 of the 31 vector files then present.
func TestValidatorAgreesWithTheDecoderOnEveryVector(t *testing.T) {
	var files []string
	for _, pat := range []string{"testdata/*/*.c4m", "testdata/*/*/*.c4m"} {
		m, _ := filepath.Glob(pat)
		files = append(files, m...)
	}
	if len(files) < 31 {
		t.Fatalf("found %d vector files, want at least 31", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d := NewDecoder(bytes.NewReader(b))
		d.quiet = true
		_, derr := d.Decode()
		for _, strict := range []bool{true, false} {
			v := NewValidator(strict)
			verr := v.ValidateManifest(bytes.NewReader(b))
			// Strict also requires canonical indentation, which the decoder
			// does not (ok-indent-3 in the name-escape vector is written by
			// the library with SetIndent(3)): that is its only addition on a
			// file the decoder reads.
			if derr == nil && strict && verr != nil && onlyIndentation(v.GetErrors()) {
				continue
			}
			if (derr != nil) != (verr != nil) {
				t.Errorf("%s (strict %v): decoder %v, Validator %v", f, strict, derr, verr)
				continue
			}
			if derr == nil {
				continue
			}
			errs := v.GetErrors()
			if last := errs[len(errs)-1]; !last.Fatal || last.Line != d.lineNum || !strings.Contains(last.Message, derr.Error()) {
				t.Errorf("%s (strict %v): want the decoder's error at line %d, got %+v", f, strict, d.lineNum, last)
			}
		}
	}
}

// libLine is the line the library writes for e alone, indented to e's
// depth: the test assembles sequences from library lines and damages only
// their order, repetition or one field, never writing a line by hand.
func libLine(t *testing.T, e Entry) string {
	t.Helper()
	depth := e.Depth
	e.Depth = 0
	m := NewManifest()
	m.AddEntry(&e)
	var b bytes.Buffer
	if err := NewEncoder(&b).Encode(m); err != nil {
		t.Fatal(err)
	}
	return strings.Repeat("  ", depth) + b.String()
}

// What the Validator adds to the decoder, each reported at its line.
func TestValidatorReportsWhatTheDecoderAccepts(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	file := func(name string, depth int) string {
		return libLine(t, Entry{Name: name, Mode: 0644, Size: 1, Timestamp: ts, Depth: depth, C4ID: c4.Identify(strings.NewReader(name))})
	}
	dir := func(name string, depth int) string {
		return libLine(t, Entry{Name: name, Mode: os.ModeDir | 0755, Size: -1, Timestamp: ts, Depth: depth})
	}
	flow := libLine(t, Entry{Name: "footage/", Mode: os.ModeDir | 0755, Size: -1, Timestamp: ts, FlowDirection: FlowOutbound, FlowTarget: "nas:../escape/"})
	cases := []struct {
		name, text string
		strict     bool
		line       int
		field      string
	}{
		{"duplicate path", file("a.txt", 0) + file("a.txt", 0), false, 2, "duplicate"},
		{"duplicate under a directory", dir("d/", 0) + file("x", 1) + file("x", 1), false, 3, "duplicate"},
		{"flow target with ..", flow, false, 1, "flow"},
		{"out of order, strict", file("b.txt", 0) + file("a.txt", 0), true, 2, "ordering"},
		{"directory before a file, strict", dir("d/", 0) + file("a.txt", 0), true, 2, "ordering"},
		{"natural order, strict", file("f10", 0) + file("f9", 0), true, 2, "ordering"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := NewValidator(c.strict)
			if err := v.ValidateManifest(strings.NewReader(c.text)); err == nil {
				t.Fatalf("accepted:\n%s", c.text)
			}
			for _, e := range v.GetErrors() {
				if e.Line == c.line && e.Field == c.field {
					return
				}
			}
			t.Errorf("want a %q error at line %d, got %v", c.field, c.line, v.GetErrors())
		})
	}

	// Lax mode accepts unsorted input (the spec: parsers MAY), and natural
	// order is canonical (f9 before f10) in both modes.
	for _, text := range []string{file("b.txt", 0) + file("a.txt", 0), file("f9", 0) + file("f10", 0)} {
		if err := NewValidator(false).ValidateManifest(strings.NewReader(text)); err != nil {
			t.Errorf("lax mode refused %q: %v", text, err)
		}
	}
	if err := NewValidator(true).ValidateManifest(strings.NewReader(file("f9", 0) + file("f10", 0))); err != nil {
		t.Errorf("strict mode refused natural order: %v", err)
	}
}

func onlyIndentation(errs []ValidationError) bool {
	for _, e := range errs {
		if e.Field != "indentation" {
			return false
		}
	}
	return len(errs) > 0
}
