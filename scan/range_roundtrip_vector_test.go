package scan

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/internal/rangevector"
)

// The folded-range round-trip vector (c4m/testdata/range-roundtrip-vector)
// is written by the library: each tree of trees.txt is built on disk,
// scanned with sequence detection (as c4 id -S scans), and encoded. This
// test regenerates every file and compares it byte for byte with the
// committed one; C4_UPDATE_VECTOR=1 rewrites them instead.
//
// A tree holding symbolic links is not written into the vector: a link's
// own time cannot be set portably, so its description is not
// reproducible. The link case is tested where it is refused (cmd/c4).
//
// The vector's trees carry POSIX permission bits (600, 640), which a
// Windows file cannot hold, so its scan there differs from the vector by
// design; Windows runs the round trip from its own scan of the same trees
// (cmd/c4).
func TestRangeRoundtripVectorIsTheLibrarys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the vector's modes are POSIX permission bits, which Windows files cannot hold")
	}
	vec := filepath.Join("..", "c4m", "testdata", "range-roundtrip-vector")
	cases, err := rangevector.Read(filepath.Join(vec, "trees.txt"))
	if err != nil {
		t.Fatal(err)
	}
	update := os.Getenv("C4_UPDATE_VECTOR") == "1"
	var ids strings.Builder
	ids.WriteString("# case\tdescription ID (the root's one-level listing; see README.md)\n")
	lists := make(map[string]string)
	for _, name := range rangevector.Names(cases) {
		if hasLinks(cases[name]) {
			continue
		}
		root := filepath.Join(t.TempDir(), name)
		if err := rangevector.Build(root, cases[name]); err != nil {
			t.Fatal(err)
		}
		m, err := Dir(root, WithMode(ModeFull), WithSequenceDetection(true))
		if err != nil {
			t.Fatalf("%s: scan: %v", name, err)
		}
		var buf bytes.Buffer
		if err := c4m.NewEncoder(&buf).Encode(m); err != nil {
			t.Fatalf("%s: encode: %v", name, err)
		}
		folded := false
		for _, e := range m.Entries {
			folded = folded || e.IsSequence
		}
		if !folded {
			t.Fatalf("%s: the scan folded no range", name)
		}
		fmt.Fprintf(&ids, "%s\t%s\n", name, m.ComputeC4ID())
		for id, list := range m.RangeData {
			lists[id.String()] = list
		}
		check(t, filepath.Join(vec, name+".c4m"), buf.Bytes(), update)
	}
	check(t, filepath.Join(vec, "ids.txt"), []byte(ids.String()), update)
	var keys []string
	for k := range lists {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		check(t, filepath.Join(vec, "lists", k), []byte(lists[k]), update)
	}
}

func hasLinks(paths []rangevector.Path) bool {
	for _, p := range paths {
		if p.Kind == 'l' {
			return true
		}
	}
	return false
}

func check(t *testing.T, file string, got []byte, update bool) {
	t.Helper()
	if update {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(file, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatalf("%v (run with C4_UPDATE_VECTOR=1 to write it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s: the library writes\n%s\nthe vector holds\n%s", file, got, want)
	}
}
