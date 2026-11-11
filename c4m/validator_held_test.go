package c4m

import (
	"strings"
	"testing"
)

// Lines the Validator must refuse but cannot yet, because the decoder reads
// them as something else and the Validator reads only through the decoder
// (design/validator-reads-through-decoder.md). Each group is the correct
// expectation, held with the reason printed until the decoder change named
// beside it lands; then delete that group's t.Skip.
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

func TestValidatorHeldForDecoderRules(t *testing.T) {
	groups := []struct {
		name, unskip string
		lines        []string
	}{
		{
			// Measured 2026-11-11: the decoder accepts these, though the
			// spec escapes a space (`\ `) and a double quote (`\"`) in a
			// name, and encodes control bytes. An unescaped space joins
			// the next token to the name, so a damaged ID field becomes
			// part of the name; an escaped control byte decodes to the
			// same name as a raw one, so only the decoder can tell them
			// apart.
			name:   "lenient name and indentation readings",
			unskip: "a ruling on whether the decoder refuses unescaped spaces, raw quotes and raw control bytes in names, and odd indentation, or reports them to the Validator",
			lines: []string{
				"-rw-r--r-- 2025-09-19T12:00:00Z 100 test.txt invalid-c4-id\n",
				"-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt c3invalidid\n",
				"-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt x4" + strings.Repeat("a", 88) + "\n",
				"-rw-r--r-- 2025-01-01T00:00:00Z 100 file\x01.txt\n",
				"drwxr-xr-x 2025-09-19T12:00:00Z 0 \"dirname\"/\n",
				"drwxr-xr-x 2025-01-01T00:00:00Z 4096 dir/\n   -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt\n",
			},
		},
	}
	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			t.Skipf("held until %s", g.unskip)
			for _, line := range g.lines {
				if err := NewValidator(true).ValidateManifest(strings.NewReader(line)); err == nil {
					t.Errorf("%q must be refused", line)
				}
			}
		})
	}
}
