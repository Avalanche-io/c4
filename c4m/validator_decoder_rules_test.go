package c4m

import (
	"strings"
	"testing"
)

// A name holding "/" other than a directory's trailing one: the decoder
// used to cut it at its first "/" and drop the rest of the line, so the
// Validator, reading through the decoder, could not see it. The name rule
// (shared name-rule vector) made the decoder refuse it, which un-held this
// group; the Validator now reports the decoder's error at line 1.
func TestValidatorRefusesNamesHoldingSlash(t *testing.T) {
	for _, line := range []string{
		"-rw-r--r-- 2025-09-19T12:00:00Z 100 sub/file.txt\n",
		"-rw-r--r-- 2025-09-19T12:00:00Z 100 foo/../bar\n",
		"drwxr-xr-x 2025-09-19T12:00:00Z 0 \"dirname/\"\n",
	} {
		v := NewValidator(true)
		if err := v.ValidateManifest(strings.NewReader(line)); err == nil {
			t.Errorf("%q must be refused", line)
			continue
		}
		if errs := v.GetErrors(); errs[len(errs)-1].Line != 1 {
			t.Errorf("%q: the error must name line 1: %v", line, errs)
		}
	}
}

// Lines the decoder used to read leniently: an unescaped space in a name
// (so a damaged C4 ID field joined the name), a raw double quote, a raw
// control byte. The name-escape rule made the decoder refuse each, which
// un-held this group. Odd indentation stays readable (the library's own
// APIs can write it) and is the Validator's: a warning, and with Strict an
// error, since canonical output indents by 2.
func TestValidatorRefusesLenientNameReadings(t *testing.T) {
	for _, line := range []string{
		"-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt invalid-c4-id\n",
		"-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c3invalidid\n",
		"-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt x4" + strings.Repeat("a", 88) + "\n",
		"-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x01.txt\n",
		"drwxr-xr-x 2025-09-19T12:00:00Z 0 \"dirname\"/\n",
	} {
		for _, strict := range []bool{true, false} {
			v := NewValidator(strict)
			if err := v.ValidateManifest(strings.NewReader(line)); err == nil {
				t.Errorf("%q (strict %v) must be refused", line, strict)
				continue
			}
			if errs := v.GetErrors(); errs[len(errs)-1].Line != 1 {
				t.Errorf("%q: the error must name line 1: %v", line, errs)
			}
		}
	}

	odd := "drwxr-xr-x 2025-01-01T00:00:00Z 4096 dir/\n   -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n"
	strict := NewValidator(true)
	if err := strict.ValidateManifest(strings.NewReader(odd)); err == nil {
		t.Error("strict: an indent unit of 3 must be an error")
	} else if errs := strict.GetErrors(); errs[0].Field != "indentation" || errs[0].Line != 2 {
		t.Errorf("strict: want an indentation error at line 2, got %v", errs)
	}
	lax := NewValidator(false)
	if err := lax.ValidateManifest(strings.NewReader(odd)); err != nil {
		t.Errorf("lax: an indent unit of 3 must be accepted with a warning, got %v", err)
	}
	if w := lax.GetWarnings(); len(w) != 1 || w[0].Field != "indentation" || w[0].Line != 2 {
		t.Errorf("lax: want one indentation warning at line 2, got %v", w)
	}
}
