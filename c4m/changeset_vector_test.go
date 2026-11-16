package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// changesetBase and changesetV2 are the two states of
// testdata/changeset-vector, built through the API: a nested tree, and the
// same tree with one file changed, one removed and one added.
func changesetBase() *Manifest {
	t1 := time.Date(2026, 1, 2, 9, 4, 5, 0, time.UTC)
	return changesetTree([]changesetEntry{
		{0, "top.txt", "top", t1},
		{0, "sub/", "", t1},
		{1, "gone.txt", "gone", t1},
		{1, "mid.txt", "mid", t1},
		{1, "deep/", "", t1},
		{2, "leaf.txt", "leaf", t1},
	})
}

func changesetV2() *Manifest {
	t1 := time.Date(2026, 1, 2, 9, 4, 5, 0, time.UTC)
	t2 := time.Date(2026, 1, 3, 10, 5, 6, 0, time.UTC)
	return changesetTree([]changesetEntry{
		{0, "top.txt", "top", t1},
		{0, "sub/", "", t2},
		{1, "mid.txt", "mid", t1},
		{1, "new.txt", "new", t2},
		{1, "deep/", "", t2},
		{2, "leaf.txt", "leaf2", t2},
	})
}

type changesetEntry struct {
	depth int
	name  string
	body  string
	ts    time.Time
}

// changesetTree builds a manifest from entries; a name ending in "/" is a
// directory, whose size and C4 ID the library derives from its children.
func changesetTree(entries []changesetEntry) *Manifest {
	m := NewManifest()
	for _, e := range entries {
		if strings.HasSuffix(e.name, "/") {
			m.AddEntry(&Entry{Mode: 0o755 | os.ModeDir, Timestamp: e.ts, Size: -1, Depth: e.depth, Name: e.name})
			continue
		}
		m.AddEntry(&Entry{Mode: 0o644, Timestamp: e.ts, Size: int64(len(e.body)), Depth: e.depth,
			Name: e.name, C4ID: c4.Identify(strings.NewReader(e.body))})
	}
	m.SortEntries()
	Normalize(m)
	return m
}

// changesetVectorFiles returns every file of testdata/changeset-vector as
// the library writes it: the two states encoded, the changeset written as
// `c4 diff` writes it, and the chain `c4 diff >>` makes.
func changesetVectorFiles(t *testing.T) map[string][]byte {
	t.Helper()
	base, v2 := changesetBase(), changesetV2()
	files := map[string][]byte{
		"base.c4m":      encodeRD(t, base),
		"v2.c4m":        encodeRD(t, v2),
		"changeset.c4m": patchText(t, PatchDiff(base, v2)),
	}
	files["chain.c4m"] = append(append([]byte{}, files["base.c4m"]...), files["changeset.c4m"]...)
	return files
}

// TestChangesetVector pins testdata/changeset-vector: every patch writer,
// given base.c4m and v2.c4m, writes changeset.c4m byte for byte, and the
// chain resolves to v2.c4m. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
// TestChangesetVector` rewrites the files from the library.
func TestChangesetVector(t *testing.T) {
	dir := filepath.Join("testdata", "changeset-vector")
	files := changesetVectorFiles(t)
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

	read := func(name string) *Manifest {
		t.Helper()
		m, err := Unmarshal(files[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return m
	}
	// The patch writer, from the files as a port reads them.
	if got := patchText(t, PatchDiff(read("base.c4m"), read("v2.c4m"))); !bytes.Equal(got, files["changeset.c4m"]) {
		t.Errorf("PatchDiff(base, v2) writes\n%s\nwant changeset.c4m:\n%s", got, files["changeset.c4m"])
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
