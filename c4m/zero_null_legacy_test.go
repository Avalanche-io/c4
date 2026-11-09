package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The legacy case in the zero-null vector: a real file written by c4
// 1.0.21 in pretty form, where that encoder wrote every null mode as ten
// dashes. Ten dashes on a DIRECTORY line can only be that legacy null (a
// correct encoder writes a 0000 directory as d---------), so it decodes
// as null. Ten dashes on a FILE line is the known zero, as ruled: a 1.0.21
// structure-level pretty file therefore reads its files' modes as 0000
// now, the accepted cost of making ten dashes mean something. And no
// encoder writes ten dashes for a null, in any form, on any line.
func TestZeroNullVectorLegacyDirectory(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "zero-null-vector", "legacy-ten-dash-1.0.21.c4m"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	dirs, files := 0, 0
	for _, e := range m.Entries {
		if e.IsDir() {
			dirs++
			if e.Mode != 0 {
				t.Errorf("%s: legacy ten-dash directory must decode as null mode, got %v", e.Name, e.Mode)
			}
			continue
		}
		files++
		if e.Mode != ZeroMode {
			t.Errorf("%s: ten dashes on a file line is the known zero, got %v", e.Name, e.Mode)
		}
	}
	if dirs == 0 || files == 0 {
		t.Fatalf("fixture must hold directories and files, got %d and %d", dirs, files)
	}

	for _, pretty := range []bool{false, true} {
		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		enc.SetPretty(pretty)
		if err := enc.Encode(m); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(buf.String(), "\n") {
			f := strings.Fields(line)
			if len(f) < 4 {
				continue
			}
			name := f[len(f)-2]
			if strings.HasSuffix(name, "/") && f[0] == "----------" {
				t.Errorf("pretty=%v: encoder wrote ten dashes for a null-mode directory: %q", pretty, line)
			}
			if !strings.HasSuffix(name, "/") && f[0] == "-" && pretty {
				// a null file mode is "-" in pretty form too
				continue
			}
		}
	}
}
