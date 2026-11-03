package c4m

import (
	"os"
	"testing"
)

// c4m/testdata/legacy/ten-dash-null-1.0.21.c4m was written by the
// released c4 1.0.21 with `c4 id -e -m s`: a structure-level listing in
// pretty form, where that encoder wrote every null mode as ten dashes,
// directories included. A correct encoder writes a zero-permission
// directory as "d---------", so ten dashes on a directory can only be a
// legacy null, and must decode as null: restoring this file must not
// chmod directories to 000.
func TestLegacyTenDashDirectoryDecodesAsNull(t *testing.T) {
	f, err := os.Open("testdata/legacy/ten-dash-null-1.0.21.c4m")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m, err := NewDecoder(f).Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var dirs int
	for _, e := range m.Entries {
		if !e.IsDir() {
			continue
		}
		dirs++
		if e.Mode != 0 {
			t.Errorf("%s: legacy ten-dash directory decoded as %v, want null", e.Name, e.Mode)
		}
		if !e.HasNullValues() {
			t.Errorf("%s: should report a null mode", e.Name)
		}
	}
	if dirs == 0 {
		t.Fatal("the legacy file should contain a directory")
	}
}
