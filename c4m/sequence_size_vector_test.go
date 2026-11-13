package c4m

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// A folded range's size is the size of every frame and of its ID list
// (ruled 2026-11-13 by the maintainer: "The size is the size of every frame
// and the frame id list. Note that the frame id list has a compleately
// deducable size based on the item count alone since c4 id's have a fixed
// length."). The list is the bare concatenation of member C4 IDs, 90 bytes
// each. A directory holding the range then follows the existing rule, with
// the range as one child: its children's sizes plus its listing as written.

// sequenceSizeFrames is sub/ holding three 11-byte frames, built through
// the API and normalised.
func sequenceSizeFrames() *Manifest {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	m := NewManifest()
	m.AddEntry(&Entry{Mode: 0o755 | os.ModeDir, Timestamp: ts, Size: -1, Name: "sub/"})
	for i := 1; i <= 3; i++ {
		body := fmt.Sprintf("frame %d abc", i) // 11 bytes
		m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: int64(len(body)), Depth: 1,
			Name: fmt.Sprintf("f.%03d.exr", i), C4ID: c4.Identify(strings.NewReader(body))})
	}
	Normalize(m)
	return m
}

// TestSequenceSizeIncludesList pins testdata/sequence-size-vector.
// `C4M_UPDATE_VECTORS=1 go test ./c4m -run TestSequenceSizeIncludesList`
// rewrites the files from the library.
func TestSequenceSizeIncludesList(t *testing.T) {
	dir := filepath.Join("testdata", "sequence-size-vector")
	frames := sequenceSizeFrames()
	folded := NewSequenceDetector(3).DetectSequences(frames)
	Normalize(folded)
	encode := func(m *Manifest) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	expanded, _, err := NewSequenceExpander(SequenceEmbedded).ExpandManifest(folded)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"frames.c4m": encode(frames), "folded.c4m": encode(folded), "expanded.c4m": encode(expanded)}
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

	var rng, sub *Entry
	for _, e := range folded.Entries {
		switch {
		case e.IsSequence:
			rng = e
		case e.Name == "sub/":
			sub = e
		}
	}
	if rng == nil || sub == nil {
		t.Fatalf("folded.c4m lacks the range or sub/")
	}
	if rng.Size != 33+3*90 {
		t.Errorf("%s: size %d, want %d (33 bytes of frames and a 3 × 90-byte ID list)", rng.Name, rng.Size, 33+3*90)
	}
	listing, _ := ListingRecord(folded.Children(sub))
	if sub.Size != rng.Size+int64(len(listing)) {
		t.Errorf("sub/: size %d, want %d (the range %d plus its listing as written, %d)", sub.Size, rng.Size+int64(len(listing)), rng.Size, len(listing))
	}
	// The normaliser derives the same: decoding folded.c4m and normalising
	// changes nothing.
	m, err := Unmarshal(files["folded.c4m"])
	if err != nil {
		t.Fatal(err)
	}
	if ch := Normalize(m); len(ch) != 0 {
		t.Errorf("the normaliser changes folded.c4m: %v", ch)
	}
	// Expanded, the frames come out with null sizes: the list names their
	// IDs and carries no per-frame metadata. The range keeps its size, which
	// counts the list.
	nFrames := 0
	for _, e := range expanded.Entries {
		switch {
		case e.IsSequence:
			if e.Size != 33+3*90 {
				t.Errorf("expanded: %s has size %d, want %d", e.Name, e.Size, 33+3*90)
			}
		case strings.HasSuffix(e.Name, ".exr"):
			nFrames++
			if !e.SizeIsNull() {
				t.Errorf("expanded: frame %s has size %d, want null", e.Name, e.Size)
			}
		}
	}
	if nFrames != 3 {
		t.Errorf("expanded: %d frames, want 3", nFrames)
	}

	// A null frame size still makes the range's size null, and sub/'s.
	nullFrame := sequenceSizeFrames()
	nullFrame.Entries[1].Size = -1
	Normalize(nullFrame)
	nf := NewSequenceDetector(3).DetectSequences(nullFrame)
	Normalize(nf)
	for _, e := range nf.Entries {
		if (e.IsSequence || e.Name == "sub/") && !e.SizeIsNull() {
			t.Errorf("one frame's size null: %s has size %d, want null", e.Name, e.Size)
		}
	}
}
