package c4m

import (
	"os"
	"strings"
	"testing"
)

// TestDeepenVectorConformance runs the shared cross-implementation fixture
// for the removal-rule narrowing (testdata/deepen-vector/README.md). Both
// files restate the same directory line byte-identically; they differ only
// in whether a child follows it, and must therefore resolve to different
// manifests.
func TestDeepenVectorConformance(t *testing.T) {
	dir := "testdata/deepen-vector/"

	cases := []struct {
		file   string
		idFile string
		want   []string
	}{
		{"deepen.c4m", "deepen-root-id.txt", []string{"a.txt", "docs/", "readme.md", "guide.md"}},
		{"remove.c4m", "remove-root-id.txt", []string{"a.txt"}},
	}

	ids := make(map[string]string)
	for _, tc := range cases {
		data, err := os.ReadFile(dir + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		// The shared fixture ends with a closing validator (a bare ID at
		// EOF naming the resolved state). The 1.0.x grammar has no closing
		// validator and reads one as an empty patch section, so it is
		// dropped here; the ID file it repeats is what the resolved
		// manifest is checked against. The fixture bytes stay identical to
		// every other implementation's copy.
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if last := lines[len(lines)-1]; strings.HasPrefix(last, "c4") && !strings.Contains(last, " ") {
			data = []byte(strings.Join(lines[:len(lines)-1], "\n") + "\n")
		}
		m, err := Unmarshal(data)
		if err != nil {
			t.Fatalf("%s must be accepted: %v", tc.file, err)
		}

		// Conformance is on the resolved entry set, not only on the error:
		// a non-verifying chain reader carrying the old rule returns the
		// wrong manifest without ever reporting a mismatch.
		got := entryNames(m)
		if len(got) != len(tc.want) {
			t.Errorf("%s resolved to %d entries, want %d: %v", tc.file, len(got), len(tc.want), got)
		}
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: %s missing from resolved manifest: %v", tc.file, w, got)
			}
		}

		wantID, err := os.ReadFile(dir + tc.idFile)
		if err != nil {
			t.Fatal(err)
		}
		gotID := m.ComputeC4ID().String()
		if gotID != strings.TrimSpace(string(wantID)) {
			t.Errorf("%s resolved to %s, want %s", tc.file, gotID, strings.TrimSpace(string(wantID)))
		}
		ids[tc.file] = gotID
	}

	// The contract in one line: the same restated directory means opposite
	// things depending on whether children follow it.
	if ids["deepen.c4m"] == ids["remove.c4m"] {
		t.Errorf("deepen and remove resolved identically (%s): the narrowing is not in effect", ids["deepen.c4m"])
	}
}
