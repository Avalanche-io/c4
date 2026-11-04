package c4m

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ListingRecord orders files before directories, each group naturally,
// and hashes the canonical lines.
func TestListingRecordOrder(t *testing.T) {
	when := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	children := []*Entry{
		{Mode: os.ModeDir | 0755, Timestamp: when, Size: 0, Name: "z/"},
		{Mode: 0644, Timestamp: when, Size: 1, Name: "b10.txt"},
		{Mode: 0644, Timestamp: when, Size: 1, Name: "b2.txt"},
		{Mode: os.ModeDir | 0755, Timestamp: when, Size: 0, Name: "a/"},
	}
	text, id := ListingRecord(children)
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var names []string
	for _, l := range lines {
		names = append(names, strings.Fields(l)[3])
	}
	if got := strings.Join(names, " "); got != "b2.txt b10.txt a/ z/" {
		t.Errorf("order = %q", got)
	}
	if id.IsNil() {
		t.Errorf("no ID")
	}
	if text2, id2 := ListingRecord(nil); text2 != "" || id2.IsNil() {
		t.Errorf("no children: want an empty record and the empty-content ID")
	}
}

// Normalize re-derives a directory's size and ID from its listed children
// and reports the stated values it replaced; its timestamp is its own
// (reading A, 2026-11-03) and is kept, filled only when null; a directory
// without listed children is kept whole.
func TestNormalizeDerivesAndReports(t *testing.T) {
	when := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	later := when.Add(time.Hour)
	m := NewManifest()
	d := &Entry{Mode: os.ModeDir | 0755, Timestamp: when, Size: 999, Name: "d/", Depth: 0}
	b := &Entry{Mode: 0644, Timestamp: later, Size: 5, Name: "b.txt", Depth: 1}
	c := &Entry{Mode: os.ModeDir | 0755, Timestamp: when, Size: 42, Name: "collapsed/", Depth: 0}
	n := &Entry{Mode: os.ModeDir | 0755, Timestamp: NullTimestamp(), Size: -1, Name: "n/", Depth: 0}
	nb := &Entry{Mode: 0644, Timestamp: later, Size: 5, Name: "b.txt", Depth: 1}
	m.Entries = append(m.Entries, d, b, c, n, nb)

	changes := Normalize(m)

	_, wantID := ListingRecord([]*Entry{b})
	if d.C4ID != wantID {
		t.Errorf("d/ ID = %v, want the listing record's %v", d.C4ID, wantID)
	}
	wantSize := int64(5 + len(b.Canonical()) + 1)
	if d.Size != wantSize {
		t.Errorf("d/ size = %d, want %d (file bytes plus the record)", d.Size, wantSize)
	}
	if !d.Timestamp.Equal(when) {
		t.Errorf("d/ timestamp = %v, want its own stated %v (children do not determine it)", d.Timestamp, when)
	}
	if !n.Timestamp.Equal(later) {
		t.Errorf("n/ null timestamp = %v, want filled from the newest child %v", n.Timestamp, later)
	}
	if c.Size != 42 || !c.Timestamp.Equal(when) {
		t.Errorf("a directory without listed children must keep its stated values")
	}
	var fields []string
	for _, ch := range changes {
		fields = append(fields, ch.Path+":"+ch.Field)
	}
	if got := strings.Join(fields, " "); got != "d/:size" {
		t.Errorf("changes = %q (a filled null is not a change; a timestamp is never replaced)", got)
	}
	if changes[0].Old != "999" || changes[0].New != sizeText(wantSize) {
		t.Errorf("size change = %+v", changes[0])
	}
}
