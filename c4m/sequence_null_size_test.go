package c4m

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// A folded sequence's size is the sum of its frames' sizes, and a frame
// whose size is unknown makes the total unknown, as a null child does a
// directory's size. Through 2026-11-13 the frame's -1 was added in: four
// 6-byte frames with one unknown folded to size 17, a known and wrong value
// (decided 2026-11-13 by the release lead; reversible).
func TestSequenceSizeIsNullWhenAFrameSizeIsNull(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	build := func(nullFrame int) *Manifest {
		m := NewManifest()
		for i := 1; i <= 4; i++ {
			body := "frame" + strconv.Itoa(i)
			size := int64(len(body))
			if i == nullFrame {
				size = -1
			}
			m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: size,
				Name: "shot." + strconv.Itoa(1000+i) + ".exr", C4ID: c4.Identify(strings.NewReader(body))})
		}
		return m
	}
	fold := func(m *Manifest) *Entry {
		t.Helper()
		out := NewSequenceDetector(3).DetectSequences(m)
		for _, e := range out.Entries {
			if e.IsSequence {
				return e
			}
		}
		t.Fatal("the frames did not fold")
		return nil
	}

	if e := fold(build(0)); e.Size != 24 {
		t.Errorf("all frames known: size %d, want 24", e.Size)
	}
	if e := fold(build(3)); !e.SizeIsNull() {
		t.Errorf("one frame's size unknown: size %d, want null", e.Size)
	}
}
