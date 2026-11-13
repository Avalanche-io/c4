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

// nullSizeVectorManifest is the description the null-size vector pins:
// every way a size of 0 can stand beside a C4 ID, built through the API.
func nullSizeVectorManifest() *Manifest {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	id := func(s string) c4.ID { return c4.Identify(strings.NewReader(s)) }
	m := NewManifest()
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: -1, Name: "null.txt", C4ID: id("hello")})
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 0, Name: "empty.txt", C4ID: EmptyContentID})
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 0, Name: "structure.txt"})
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 5, Name: "known.txt", C4ID: id("hello")})
	m.AddEntry(&Entry{Mode: os.ModeSymlink | 0o755, Timestamp: ts, Size: 0, Name: "link", Target: "known.txt"})
	m.AddEntry(&Entry{Mode: os.ModeDir | 0o755, Timestamp: ts, Size: 0, Name: "emptydir/", C4ID: EmptyContentID})
	m.AddEntry(&Entry{Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1, Name: "collapsed/", C4ID: id("a listing")})
	return m
}

// zeroSpelled is canonical with the size field of every null-size line
// written as 0, which is how a filesystem surface with no "-" shows a
// null size. It is the one transformation applied to the library's
// output; no line is composed by hand.
func zeroSpelled(t *testing.T, canonical []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, line := range strings.SplitAfter(string(canonical), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, " ", 4) // mode, timestamp, size, rest
		if len(f) == 4 && f[2] == "-" {
			line = f[0] + " " + f[1] + " 0 " + f[3]
		}
		out.WriteString(line)
	}
	if bytes.Equal(out.Bytes(), canonical) {
		t.Fatal("the vector must hold at least one null size")
	}
	return out.Bytes()
}

// TestNullSizeVector pins testdata/null-size-vector: a size of 0 beside a
// C4 ID other than the empty content's is a null size (ruled 2026-11-08;
// landed 2026-11-13, decided by the release lead; reversible).
// `C4M_UPDATE_VECTORS=1 go test ./c4m -run TestNullSizeVector` rewrites the
// files from the library.
func TestNullSizeVector(t *testing.T) {
	dir := filepath.Join("testdata", "null-size-vector")
	m := nullSizeVectorManifest()

	var canon bytes.Buffer
	if err := NewEncoder(&canon).Encode(m); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"canonical.c4m":    canon.Bytes(),
		"zero-spelled.c4m": zeroSpelled(t, canon.Bytes()),
		"root-id.txt":      []byte(m.ComputeC4ID().String() + "\n"),
	}
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

	wantNull := map[string]bool{
		"null.txt": true, "empty.txt": false, "structure.txt": false, "known.txt": false,
		"link": false, "emptydir/": false, "collapsed/": true,
	}
	root := strings.TrimSpace(string(files["root-id.txt"]))
	for _, name := range []string{"canonical.c4m", "zero-spelled.c4m"} {
		got, err := Unmarshal(files[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if id := got.ComputeC4ID().String(); id != root {
			t.Errorf("%s resolves to %s, want %s", name, id, root)
		}
		for _, e := range got.Entries {
			if w, ok := wantNull[e.Name]; !ok || e.SizeIsNull() != w {
				t.Errorf("%s: %s: null size %v, want %v", name, e.Name, e.SizeIsNull(), w)
			}
		}
		// Re-encoding writes each null size as "-": both files encode to
		// the canonical bytes.
		var re bytes.Buffer
		if err := NewEncoder(&re).Encode(got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(re.Bytes(), files["canonical.c4m"]) {
			t.Errorf("%s re-encodes to\n%s\nwant\n%s", name, re.Bytes(), files["canonical.c4m"])
		}
	}
}
