package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// rangeDataVectorFiles returns every file of testdata/range-data-vector, as
// the library writes it. The inputs are built through the API
// (rangeDataFolded, rangeDataPlain); every other file is an operation's
// output, and a chain is two library outputs one after the other, as
// `c4 diff >>` makes one.
func rangeDataVectorFiles(t *testing.T) map[string][]byte {
	t.Helper()
	folded := rangeDataFolded()
	plain := rangeDataPlain()
	noPlate := withoutRange(folded, "plate.[1001-1004].exr")
	merged, conflicts, err := Merge(nil, folded, plain)
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("merge: %v, %d conflicts", err, len(conflicts))
	}
	files := map[string][]byte{
		"folded.c4m":   encodeRD(t, folded),
		"plain.c4m":    encodeRD(t, plain),
		"no-plate.c4m": encodeRD(t, noPlate),
		"merged.c4m":   encodeRD(t, merged),
		"add.c4m":      patchText(t, PatchDiff(plain, folded)),
		"remove.c4m":   patchText(t, PatchDiff(folded, noPlate)),
	}
	files["add-chain.c4m"] = append(append([]byte{}, files["plain.c4m"]...), files["add.c4m"]...)
	files["remove-chain.c4m"] = append(append([]byte{}, files["folded.c4m"]...), files["remove.c4m"]...)
	return files
}

// TestRangeDataVector pins testdata/range-data-vector: merge, diff and
// patch keep the ID lists of the ranges their output holds, and no other
// (design/range-data-through-operations.md).
// `C4M_UPDATE_VECTORS=1 go test ./c4m -run TestRangeDataVector` rewrites
// the files from the library.
func TestRangeDataVector(t *testing.T) {
	dir := filepath.Join("testdata", "range-data-vector")
	files := rangeDataVectorFiles(t)
	if os.Getenv("C4M_UPDATE_VECTORS") != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from what the library writes:\n--- file\n%s--- library\n%s", name, got, want)
		}
	}

	// The operations, run on the files as a port reads them.
	read := func(name string) *Manifest {
		t.Helper()
		m, err := Unmarshal(files[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return m
	}
	same := func(what string, m *Manifest, want string) {
		t.Helper()
		if got := encodeRD(t, m); !bytes.Equal(got, files[want]) {
			t.Errorf("%s writes\n%s\nwant %s:\n%s", what, got, want, files[want])
		}
	}
	merge := func(a, b string) *Manifest {
		t.Helper()
		m, _, err := Merge(nil, read(a), read(b))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	same("Merge(folded, plain)", merge("folded.c4m", "plain.c4m"), "merged.c4m")
	same("Merge(plain, folded)", merge("plain.c4m", "folded.c4m"), "merged.c4m")
	same("Merge(folded, folded)", merge("folded.c4m", "folded.c4m"), "folded.c4m")
	if got := patchText(t, PatchDiff(read("plain.c4m"), read("folded.c4m"))); !bytes.Equal(got, files["add.c4m"]) {
		t.Errorf("PatchDiff(plain, folded) writes\n%s\nwant add.c4m", got)
	}
	if got := patchText(t, PatchDiff(read("folded.c4m"), read("no-plate.c4m"))); !bytes.Equal(got, files["remove.c4m"]) {
		t.Errorf("PatchDiff(folded, no-plate) writes\n%s\nwant remove.c4m", got)
	}
	for chain, want := range map[string]string{"add-chain.c4m": "folded.c4m", "remove-chain.c4m": "no-plate.c4m"} {
		same("Decode("+chain+")", read(chain), want)
		secs, err := DecodePatchChain(bytes.NewReader(files[chain]))
		if err != nil {
			t.Fatal(err)
		}
		same("ResolvePatchChain("+chain+")", ResolvePatchChain(secs, 0), want)
	}

	// The lists are transport data, never identity: dropping them changes
	// a file's bytes and byte ID, never its description ID.
	for _, name := range []string{"folded.c4m", "merged.c4m", "no-plate.c4m"} {
		m := read(name)
		if len(m.RangeData) == 0 {
			t.Errorf("%s carries no ID list", name)
		}
		bare := m.Copy()
		bare.RangeData = nil
		if m.ComputeC4ID() != bare.ComputeC4ID() {
			t.Errorf("%s: its ID lists change its description ID", name)
		}
	}
}
