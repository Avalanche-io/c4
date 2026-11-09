package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestZeroNullVector pins testdata/zero-null-vector, the shared
// conformance fixture for known zero against null (see its README). The
// files are made by the library on master (TestZeroNullVector there
// regenerates them); here they are the contract: both forms decode to one
// manifest with the expected knowledge per entry and the root ID in
// root-id.txt, and the canonical form re-encodes byte for byte.
func TestZeroNullVector(t *testing.T) {
	dir := filepath.Join("testdata", "zero-null-vector")
	rootID, err := os.ReadFile(filepath.Join(dir, "root-id.txt"))
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		modeNull, modeZero, sizeNull, idNil bool
		size                                int64
	}
	expect := map[string]want{
		"zero.txt":     {modeZero: true, size: 4},
		"nullmode.txt": {modeNull: true, size: 4},
		"empty.txt":    {size: 0},
		"emptydir/":    {size: 0},
		"locked/":      {size: 142},
		"inside.txt":   {size: 6},
		"unread/":      {sizeNull: true, idNil: true},
	}
	for _, form := range []string{"canonical.c4m", "pretty.c4m"} {
		data, err := os.ReadFile(filepath.Join(dir, form))
		if err != nil {
			t.Fatal(err)
		}
		dec, err := Unmarshal(data)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if got := dec.ComputeC4ID().String(); got != strings.TrimSpace(string(rootID)) {
			t.Errorf("%s resolves to %s, want %s", form, got, strings.TrimSpace(string(rootID)))
		}
		seen := 0
		for _, e := range dec.Entries {
			w, ok := expect[e.Name]
			if !ok {
				t.Errorf("%s: unexpected entry %s", form, e.Name)
				continue
			}
			seen++
			if (e.Mode == 0) != w.modeNull {
				t.Errorf("%s: %s mode null = %v, want %v", form, e.Name, e.Mode == 0, w.modeNull)
			}
			if (e.Mode == ZeroMode) != w.modeZero {
				t.Errorf("%s: %s mode zero = %v, want %v", form, e.Name, e.Mode == ZeroMode, w.modeZero)
			}
			if (e.Name == "locked/" || e.Name == "unread/") && (e.Mode == 0 || e.Mode.Perm() != 0 || !e.IsDir()) {
				t.Errorf("%s: %s must be d--------- (known, zero permissions)", form, e.Name)
			}
			if (e.Size < 0) != w.sizeNull {
				t.Errorf("%s: %s size null = %v, want %v (size %d)", form, e.Name, e.Size < 0, w.sizeNull, e.Size)
			}
			if !w.sizeNull && e.Size != w.size {
				t.Errorf("%s: %s size %d, want %d", form, e.Name, e.Size, w.size)
			}
			if e.C4ID.IsNil() != w.idNil {
				t.Errorf("%s: %s ID nil = %v, want %v", form, e.Name, e.C4ID.IsNil(), w.idNil)
			}
		}
		if seen != len(expect) {
			t.Errorf("%s: %d of %d entries seen", form, seen, len(expect))
		}
		if form == "canonical.c4m" {
			var buf bytes.Buffer
			if err := NewEncoder(&buf).Encode(dec); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf.Bytes(), data) {
				t.Errorf("canonical re-encode differs:\n--- file\n%s--- encoder\n%s", data, buf.String())
			}
		}
	}
}
