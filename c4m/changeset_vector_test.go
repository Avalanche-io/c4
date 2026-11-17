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
	for _, name := range []string{"base.c4m", "v2.c4m", "v3.c4m"} {
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
	files["changeset-v3.c4m"] = patchText(t, PatchDiff(read("v2.c4m"), read("v3.c4m")))
	files["chain.c4m"] = append(append([]byte{}, files["base.c4m"]...), files["changeset.c4m"]...)
	files["chain-v3.c4m"] = append(append([]byte{}, files["v2.c4m"]...), files["changeset-v3.c4m"]...)
	return files
}

// changesetSteps are the vector's two changes: each names its old and new
// states, the changeset between them and the chain that appends it.
var changesetSteps = []struct{ old, new, changeset, chain string }{
	{"base.c4m", "v2.c4m", "changeset.c4m", "chain.c4m"},
	{"v2.c4m", "v3.c4m", "changeset-v3.c4m", "chain-v3.c4m"},
}

// TestChangesetVector pins testdata/changeset-vector: every patch writer,
// given base.c4m and v2.c4m, writes changeset.c4m byte for byte, and the
// chain resolves to v2.c4m. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
// TestChangesetVector` rewrites the changesets and chains from the library
// (scan's TestChangesetVectorStates rewrites base.c4m, v2.c4m and v3.c4m).
func TestChangesetVector(t *testing.T) {
	dir := filepath.Join("testdata", "changeset-vector")
	files := changesetVectorFiles(t, dir)
	if os.Getenv("C4M_UPDATE_VECTORS") != "" {
		for _, name := range []string{"changeset.c4m", "chain.c4m", "changeset-v3.c4m", "chain-v3.c4m"} {
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
	for _, st := range changesetSteps {
		// A changeset never ends with a bare C4 ID.
		lines := strings.Split(strings.TrimRight(string(files[st.changeset]), "\n"), "\n")
		if last := lines[len(lines)-1]; len(last) == 90 && strings.HasPrefix(last, "c4") {
			t.Errorf("%s ends with a bare C4 ID: %s", st.changeset, last)
		}
		// The chain, through the decoder and the chain readers.
		if got := encodeRD(t, read(st.chain)); !bytes.Equal(got, files[st.new]) {
			t.Errorf("Decode(%s) writes\n%s\nwant %s", st.chain, got, st.new)
		}
		secs, err := DecodePatchChain(bytes.NewReader(files[st.chain]))
		if err != nil {
			t.Fatal(err)
		}
		if got := encodeRD(t, ResolvePatchChain(secs, 0)); !bytes.Equal(got, files[st.new]) {
			t.Errorf("ResolvePatchChain(%s) writes\n%s\nwant %s", st.chain, got, st.new)
		}
		// Applying the changeset's entries to the old state.
		patch, err := Unmarshal([]byte(strings.SplitN(string(files[st.changeset]), "\n", 2)[1]))
		if err != nil {
			t.Fatal(err)
		}
		if got := encodeRD(t, ApplyPatch(read(st.old), patch)); !bytes.Equal(got, files[st.new]) {
			t.Errorf("ApplyPatch(%s, %s) writes\n%s\nwant %s", st.old, st.changeset, got, st.new)
		}
	}
	// The README's Round trip table on this line: a decode keeps a
	// changeset's base line and an encode drops it (keeping it is c4 #145,
	// 1.1), so each changeset re-encodes to its text without its first line.
	withoutBase := func(name string) []byte { return []byte(strings.SplitN(string(files[name]), "\n", 2)[1]) }
	roundTrip := map[string][]byte{
		"base.c4m":         files["base.c4m"],
		"v2.c4m":           files["v2.c4m"],
		"v3.c4m":           files["v3.c4m"],
		"changeset.c4m":    withoutBase("changeset.c4m"),
		"changeset-v3.c4m": withoutBase("changeset-v3.c4m"),
		"chain.c4m":        files["v2.c4m"],
		"chain-v3.c4m":     files["v3.c4m"],
	}
	for name, want := range roundTrip {
		if got := encodeRD(t, read(name)); !bytes.Equal(got, want) {
			t.Errorf("%s re-encodes to\n%s\nwant, as the README's Round trip table says:\n%s", name, got, want)
		}
	}
	// A removed directory is its own line alone: nothing under sub/deep/ is
	// restated, since a directory restated with children is deepened.
	if bytes.Contains(files["changeset-v3.c4m"], []byte("leaf.txt")) {
		t.Errorf("changeset-v3.c4m restates a child of the removed sub/deep/:\n%s", files["changeset-v3.c4m"])
	}
}
