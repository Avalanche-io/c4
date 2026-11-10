package c4m

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// A pretty line with a null timestamp keeps its alignment padding, and
// the target parser took that padding into a link's target, escaped it
// on the next write, and so changed the tree's ID with no error. c4 id -e
// reaches it on any tree holding a symlink whose own mtime is the epoch
// (measured on 1.0.24, 2026-11-10). Each entry is built through the API.
func TestPrettyPaddingIsNotPartOfALinkTarget(t *testing.T) {
	defer func(l *time.Location) { time.Local = l }(time.Local)
	time.Local = time.UTC
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	content := c4.Identify(strings.NewReader("what the link points at"))
	idText := c4.Identify(strings.NewReader("some content")).String()
	link := func(name, target string, stamp time.Time, id c4.ID) *Entry {
		return &Entry{Name: name, Mode: os.ModeSymlink | 0o755, Size: int64(len(target)), Timestamp: stamp, Target: target, C4ID: id}
	}
	m := NewManifest()
	m.AddEntry(link("epoch-link-nil", "missing.txt", NullTimestamp(), c4.ID{}))
	m.AddEntry(link("epoch-link-id", "real.txt", NullTimestamp(), content))
	m.AddEntry(link("objects-nil", "../objects/"+idText, NullTimestamp(), c4.ID{}))
	m.AddEntry(link("dated-nil", "a.txt", ts, c4.ID{}))
	m.AddEntry(&Entry{Name: "outbox/", Mode: os.ModeDir | 0o755, Size: -1, Timestamp: NullTimestamp(),
		FlowDirection: FlowOutbound, FlowTarget: "studio:inbox/"})
	m.SortEntries()

	var canon, pretty bytes.Buffer
	if err := NewEncoder(&canon).Encode(m); err != nil {
		t.Fatal(err)
	}
	pe := NewEncoder(&pretty)
	pe.SetPretty(true)
	if err := pe.Encode(m); err != nil {
		t.Fatal(err)
	}
	want := m.ComputeC4ID()
	for form, text := range map[string][]byte{"canonical": canon.Bytes(), "pretty": pretty.Bytes()} {
		back, err := Unmarshal(text)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		byName := map[string]*Entry{}
		for _, e := range back.Entries {
			byName[e.Name] = e
		}
		for _, e := range m.Entries {
			got := byName[e.Name]
			if got == nil || got.Target != e.Target || got.FlowTarget != e.FlowTarget || got.C4ID != e.C4ID {
				t.Errorf("%s: %s read back as %+v", form, e.Name, got)
			}
		}
		if id := back.ComputeC4ID(); id != want {
			t.Errorf("%s: read back to a different ID %s, want %s", form, id, want)
		}
	}
}
