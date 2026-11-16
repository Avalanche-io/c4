package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On the 1.0.x line the library derives a directory's C4 ID only by
// scanning, so base.c4m and v2.c4m are written by the scanner on a tree on
// disk and pinned by scan's TestChangesetVectorStates. Here they are read
// from the files, and the changeset and chain are what the library writes
// from them: PatchDiff written as `c4 diff` writes it, and the chain
// `c4 diff >>` makes. Master builds base and v2 through the API; the four
// files are byte-identical on both lines.
func changesetVectorFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range []string{"base.c4m", "v2.c4m"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = b
	}
	read := func(name string) *Manifest {
		t.Helper()
		m, err := Unmarshal(files[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return m
	}
	files["changeset.c4m"] = patchText(t, PatchDiff(read("base.c4m"), read("v2.c4m")))
	files["chain.c4m"] = append(append([]byte{}, files["base.c4m"]...), files["changeset.c4m"]...)
	return files
}

// TestChangesetVector pins testdata/changeset-vector: every patch writer,
// given base.c4m and v2.c4m, writes changeset.c4m byte for byte, and the
// chain resolves to v2.c4m. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
// TestChangesetVector` rewrites changeset.c4m and chain.c4m from the
// library (scan's TestChangesetVectorStates rewrites base.c4m and v2.c4m).
func TestChangesetVector(t *testing.T) {
	dir := filepath.Join("testdata", "changeset-vector")
	files := changesetVectorFiles(t, dir)
	if os.Getenv("C4M_UPDATE_VECTORS") != "" {
		for _, name := range []string{"changeset.c4m", "chain.c4m"} {
			if err := os.WriteFile(filepath.Join(dir, name), files[name], 0o644); err != nil {
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

	read := func(name string) *Manifest {
		t.Helper()
		m, err := Unmarshal(files[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return m
	}
	// A changeset never ends with a bare C4 ID.
	lines := strings.Split(strings.TrimRight(string(files["changeset.c4m"]), "\n"), "\n")
	if last := lines[len(lines)-1]; len(last) == 90 && strings.HasPrefix(last, "c4") {
		t.Errorf("changeset.c4m ends with a bare C4 ID: %s", last)
	}
	// The chain, through the decoder and the chain readers.
	if got := encodeRD(t, read("chain.c4m")); !bytes.Equal(got, files["v2.c4m"]) {
		t.Errorf("Decode(chain.c4m) writes\n%s\nwant v2.c4m", got)
	}
	secs, err := DecodePatchChain(bytes.NewReader(files["chain.c4m"]))
	if err != nil {
		t.Fatal(err)
	}
	if got := encodeRD(t, ResolvePatchChain(secs, 0)); !bytes.Equal(got, files["v2.c4m"]) {
		t.Errorf("ResolvePatchChain(chain.c4m) writes\n%s\nwant v2.c4m", got)
	}
	// Applying the changeset's entries to base.
	patch, err := Unmarshal([]byte(strings.SplitN(string(files["changeset.c4m"]), "\n", 2)[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got := encodeRD(t, ApplyPatch(read("base.c4m"), patch)); !bytes.Equal(got, files["v2.c4m"]) {
		t.Errorf("ApplyPatch(base, changeset) writes\n%s\nwant v2.c4m", got)
	}
}
