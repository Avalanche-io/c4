package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// sequenceNullSizeFrames is the description the sequence null-size vector
// pins, built through the API: two runs of four frames, one with a frame
// whose size is unknown and one with every size known.
func sequenceNullSizeFrames() *Manifest {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	m := NewManifest()
	for i := 1; i <= 4; i++ {
		body := "plate" + strconv.Itoa(i)
		m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: int64(len(body)),
			Name: "plate." + strconv.Itoa(1000+i) + ".exr", C4ID: c4.Identify(strings.NewReader(body))})
	}
	for i := 1; i <= 4; i++ {
		body := "shot" + strconv.Itoa(i)
		size := int64(len(body))
		if i == 3 {
			size = -1
		}
		m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: size,
			Name: "shot." + strconv.Itoa(1000+i) + ".exr", C4ID: c4.Identify(strings.NewReader(body))})
	}
	return m
}

// TestSequenceNullSizeVector pins testdata/sequence-null-size-vector: a
// folded sequence with a frame whose size is null has a null size (ruled
// 2026-11-13 by the maintainer: "One cannot use size as an expectation if
// there are nulls present, so we must say nothing").
// `C4M_UPDATE_VECTORS=1 go test ./c4m -run TestSequenceNullSizeVector`
// rewrites the files from the library.
func TestSequenceNullSizeVector(t *testing.T) {
	dir := filepath.Join("testdata", "sequence-null-size-vector")
	frames := sequenceNullSizeFrames()
	folded := NewSequenceDetector(3).DetectSequences(frames)

	encode := func(m *Manifest) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	framesText := encode(frames)
	files := map[string][]byte{
		"frames.c4m":              framesText,
		"frames-zero-spelled.c4m": zeroSpelled(t, framesText),
		"folded.c4m":              encode(folded),
		"folded-root-id.txt":      []byte(folded.ComputeC4ID().String() + "\n"),
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

	// frames.c4m folds to folded.c4m, and the folded text decodes to a
	// null-size shot range and a known-size plate range.
	root := strings.TrimSpace(string(files["folded-root-id.txt"]))
	m, err := Unmarshal(files["frames.c4m"])
	if err != nil {
		t.Fatal(err)
	}
	if got := encode(NewSequenceDetector(3).DetectSequences(m)); !bytes.Equal(got, files["folded.c4m"]) {
		t.Errorf("frames.c4m folds to\n%s\nwant\n%s", got, files["folded.c4m"])
	}

	// 1.0.x: the zero-versus-null rule is held off this line, so the 0 in
	// frames-zero-spelled.c4m is a known size here, not a null one, and
	// the shot range folds to a known 15 (5 + 5 + 0 + 5). Master folds it
	// to folded.c4m; the file's bytes are master's, byte for byte.
	m, err = Unmarshal(files["frames-zero-spelled.c4m"])
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range NewSequenceDetector(3).DetectSequences(m).Entries {
		if e.Name == "shot.[1001-1004].exr" && e.Size != 15 {
			t.Errorf("frames-zero-spelled.c4m: shot range size %d, want 15 on 1.0.x (0 is a known size here)", e.Size)
		}
	}
	m, err = Unmarshal(files["folded.c4m"])
	if err != nil {
		t.Fatal(err)
	}
	if id := m.ComputeC4ID().String(); id != root {
		t.Errorf("folded.c4m resolves to %s, want %s", id, root)
	}
	want := map[string]int64{"plate.[1001-1004].exr": 24, "shot.[1001-1004].exr": -1}
	for _, e := range m.Entries {
		w, ok := want[e.Name]
		if !ok {
			continue
		}
		delete(want, e.Name)
		if w < 0 && e.Size >= 0 || w >= 0 && e.Size != w {
			t.Errorf("%s: size %d, want %d (-1 is null)", e.Name, e.Size, w)
		}
	}
	if len(want) != 0 {
		t.Errorf("folded.c4m lacks %v", want)
	}
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
