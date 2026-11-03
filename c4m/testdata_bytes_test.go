package c4m

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// Test vectors are content-addressed bytes: git must never transform them
// (.gitattributes: c4m/testdata/** -text) and a checkout on any platform
// must give the same bytes. This pins the two ways that fails: a CR that
// crept in through a line-ending conversion, and a vector whose ID moved.
// A vector that carries a CR on purpose names it in its file name.
func TestTestVectorsAreByteStable(t *testing.T) {
	pinned := map[string]string{
		// written by the released c4 1.0.21 with `c4 id -e -m s`
		"legacy/ten-dash-null-1.0.21.c4m": "c4133mQ12aFA8wj1H63wAJvSfRB3CREF4xJeZ7Sxw5cdgneHsjhtrHqbVgQcCzTC5r1YWD219YBjsQL6Zfr5UCGkq9",
	}
	seen := 0
	err := filepath.Walk("testdata", func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, "testdata/"))
		if bytes.IndexByte(data, '\r') >= 0 && !strings.Contains(rel, "crlf") {
			t.Errorf("%s carries a CR: a line-ending conversion changed its bytes (or name it *crlf* if meant)", rel)
		}
		if want, ok := pinned[rel]; ok {
			seen++
			if got := c4.Identify(bytes.NewReader(data)).String(); got != want {
				t.Errorf("%s: ID %s, pinned %s: the vector's bytes changed", rel, got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != len(pinned) {
		t.Errorf("%d of %d pinned vectors found", seen, len(pinned))
	}
}
