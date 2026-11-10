package c4m_test

// A symlink whose target is the text of a C4 ID, written by the c4m
// library and read back by it. From the storage project's review,
// 2026-11-10, kept as it arrived: it ran red on 4b1fe0a. Every entry is
// built through the package's API; no c4m text is written by hand.

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func link(target string, id c4.ID) *c4m.Entry {
	return &c4m.Entry{Mode: os.ModeSymlink | 0o777, Timestamp: time.Unix(0, 0).UTC(), Size: int64(len(target)), Name: "link", Target: target, C4ID: id}
}

func TestASymlinkTargetThatIsAC4ID(t *testing.T) {
	idText := c4.Identify(strings.NewReader("some content")).String()
	content := c4.Identify(strings.NewReader("what the link points at"))
	for _, c := range []struct {
		name   string
		target string
		id     c4.ID
	}{
		{"an ordinary target, no C4 ID", "a.txt", c4.ID{}},
		{"an ordinary target, a C4 ID", "a.txt", content},
		{"a C4 ID as the target, no C4 ID", idText, c4.ID{}},
		{"a C4 ID as the target, a C4 ID", idText, content},
		{"a C4 ID and more as the target, no C4 ID", idText + ".exr", c4.ID{}},
		{"a C4 ID in a path as the target, no C4 ID", "../objects/" + idText, c4.ID{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := c4m.NewManifest()
			m.Entries = []*c4m.Entry{link(c.target, c.id)}
			text, err := c4m.Marshal(m)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			t.Logf("written: %q", strings.TrimSpace(string(text)))
			back, err := c4m.Unmarshal(text)
			if err != nil {
				t.Fatalf("FAILS: the library cannot read back the line it wrote: %v", err)
			}
			if len(back.Entries) != 1 {
				t.Fatalf("FAILS: read back as %d entries", len(back.Entries))
			}
			e := back.Entries[0]
			if e.Target != c.target || e.C4ID != c.id || e.Name != "link" {
				t.Fatalf("FAILS: read back as name %q target %q C4 ID %v", e.Name, e.Target, e.C4ID)
			}
			again, _ := c4m.Marshal(back)
			if !bytes.Equal(again, text) {
				t.Fatalf("FAILS: written again it is %q", again)
			}
		})
	}
}

// The entry as the operator's views carry a symlink: mode, timestamp,
// size and C4 ID all null, the target real. Written as a manifest, and as
// a changeset (a patch over a base view, the form the git importer
// stores and a chain file joins).
func TestASymlinkTargetThatIsAC4IDWithNullFields(t *testing.T) {
	idText := c4.Identify(strings.NewReader("some content")).String()
	base := c4.Identify(strings.NewReader("a base view"))
	for _, target := range []string{"a.txt", idText, idText + ".exr", "../objects/" + idText} {
		e := &c4m.Entry{Name: "link", Size: -1, Target: target, Timestamp: c4m.NullTimestamp()}
		t.Run("manifest/"+target[:5], func(t *testing.T) {
			m := c4m.NewManifest()
			m.Entries = []*c4m.Entry{e}
			text, err := c4m.Marshal(m)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			t.Logf("written: %q", strings.TrimSpace(string(text)))
			back, err := c4m.Unmarshal(text)
			if err != nil {
				t.Fatalf("FAILS: the library cannot read back the line it wrote: %v", err)
			}
			if got := back.Entries[0]; got.Target != target || !got.C4ID.IsNil() || got.Name != "link" {
				t.Fatalf("FAILS: read back as name %q target %q C4 ID %v", got.Name, got.Target, got.C4ID)
			}
		})
		t.Run("changeset/"+target[:5], func(t *testing.T) {
			m := c4m.NewManifest()
			m.Entries = []*c4m.Entry{e}
			var buf bytes.Buffer
			if err := c4m.NewEncoder(&buf).EncodePatch(&c4m.PatchResult{Patch: m, OldID: base}); err != nil {
				t.Fatalf("EncodePatch: %v", err)
			}
			t.Logf("written: %q", buf.String())
			sections, err := c4m.DecodePatchChain(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("FAILS: the library cannot read back the changeset it wrote: %v", err)
			}
			if len(sections) != 1 || len(sections[0].Entries) != 1 {
				t.Fatalf("FAILS: read back as %d sections", len(sections))
			}
			if got := sections[0].Entries[0]; got.Target != target || !got.C4ID.IsNil() || got.Name != "link" {
				t.Fatalf("FAILS: read back as name %q target %q C4 ID %v", got.Name, got.Target, got.C4ID)
			}
		})
	}
}
