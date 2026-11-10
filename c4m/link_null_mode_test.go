package c4m

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// Every link the library writes must read back as the same entry, in
// both the plain form and a patch, whatever the mode. With a null mode
// the line does not say the entry is a symlink, so the decoder must tell
// a symlink target from an ungrouped hard link's C4 ID and from a flow
// target by the fields alone (found by the storage project's git
// importer, 2026-11-10: a symlink whose target is a C4 ID's text, with a
// null mode and C4 ID, was written and then refused on read).
func TestEveryLinkReadsBackWhateverTheMode(t *testing.T) {
	runLinkRoundTrips(t, []string{
		"a.txt", "c4", "c4studio", "-", "a b",
		c4.Identify(strings.NewReader("some content")).String(),
		c4.Identify(strings.NewReader("some content")).String() + ".exr",
		"../objects/" + c4.Identify(strings.NewReader("some content")).String(),
	}, true)
}

// A symlink whose target has the shape of a flow target (a location
// label and a colon: "host:path", "C:foo", "studio:inbox/") cannot be
// told from a flow link when the mode is null: both are "-> X <c4id>",
// and the spec's disambiguation (SPECIFICATION.md "Flow Links") assumes
// the mode says symlink. Today the decoder reads it as a flow link, a
// different entry, with no error. Which reading is right is a format
// question awaiting a ruling, so these cases are recorded, not pinned;
// under a symlink mode they read back and run in the test above.
func TestNullModeLinkWithAFlowShapedTargetIsUnruled(t *testing.T) {
	t.Skip("unruled (2026-11-10): a null-mode link whose target is location-shaped reads as a flow link")
	runLinkRoundTrips(t, []string{"host:path", "C:foo", "studio:inbox/"}, false)
}

func runLinkRoundTrips(t *testing.T, targets []string, withOthers bool) {
	t.Helper()
	content := c4.Identify(strings.NewReader("what the link points at"))
	base := c4.Identify(strings.NewReader("a base view"))

	type linkCase struct {
		name  string
		entry Entry
	}
	symlink := func(target string, id c4.ID, mode os.FileMode) Entry {
		return Entry{Name: "link", Mode: mode, Size: -1, Timestamp: NullTimestamp(), Target: target, C4ID: id}
	}
	var cases []linkCase
	for _, mode := range []struct {
		label string
		mode  os.FileMode
	}{{"null mode", 0}, {"symlink mode", os.ModeSymlink | 0o777}} {
		for _, target := range targets {
			for _, id := range []c4.ID{{}, content} {
				label := "no C4 ID"
				if !id.IsNil() {
					label = "C4 ID"
				}
				cases = append(cases, linkCase{
					name:  mode.label + "/target " + shortLabel(target) + "/" + label,
					entry: symlink(target, id, mode.mode),
				})
			}
		}
	}
	for _, id := range []c4.ID{{}, content} {
		if !withOthers {
			break
		}
		for _, group := range []int{-1, 2} {
			cases = append(cases, linkCase{
				name:  "null mode/hard link " + groupLabel(group, id),
				entry: Entry{Name: "file", Size: -1, Timestamp: NullTimestamp(), HardLink: group, C4ID: id},
			})
		}
	}
	if withOthers {
		// The symlink-mode form of the flow-shaped targets is pinned:
		// with the mode saying symlink there is no ambiguity.
		for _, target := range []string{"host:path", "C:foo", "studio:inbox/"} {
			cases = append(cases, linkCase{
				name:  "symlink mode/target " + target,
				entry: symlink(target, c4.ID{}, os.ModeSymlink|0o777),
			})
		}
	}
	cases = append(cases, linkCase{
		name: "null mode/flow link on a directory",
		entry: Entry{Name: "outbox/", Size: -1, Timestamp: NullTimestamp(),
			FlowDirection: FlowOutbound, FlowTarget: "studio:inbox/"},
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.entry

			for _, pretty := range []bool{false, true} {
				m := NewManifest()
				e := want
				m.Entries = []*Entry{&e}
				var buf bytes.Buffer
				enc := NewEncoder(&buf)
				enc.SetPretty(pretty)
				if err := enc.Encode(m); err != nil {
					t.Fatalf("Encode: %v", err)
				}
				text := buf.Bytes()
				back, err := Unmarshal(text)
				if err != nil {
					t.Fatalf("pretty=%v written %q, read back: %v", pretty, strings.TrimSpace(string(text)), err)
				}
				if len(back.Entries) != 1 || !sameLink(back.Entries[0], &want) {
					t.Fatalf("pretty=%v written %q, read back as %+v", pretty, strings.TrimSpace(string(text)), back.Entries)
				}
				if pretty {
					continue // pretty padding depends on the widest line; canonical is the byte-stable form
				}
				if again, _ := Marshal(back); !bytes.Equal(again, text) {
					t.Fatalf("written %q, written again %q", text, again)
				}
			}

			var patch bytes.Buffer
			e2 := want
			pr := &PatchResult{OldID: base, Patch: &Manifest{Version: "1.0", Entries: []*Entry{&e2}}}
			if err := NewEncoder(&patch).EncodePatch(pr); err != nil {
				t.Fatalf("EncodePatch: %v", err)
			}
			sections, err := DecodePatchChain(bytes.NewReader(patch.Bytes()))
			if err != nil {
				t.Fatalf("patch %q, read back: %v", patch.String(), err)
			}
			if len(sections) != 1 || len(sections[0].Entries) != 1 || !sameLink(sections[0].Entries[0], &want) {
				t.Fatalf("patch %q, read back as %+v", patch.String(), sections)
			}
		})
	}
}

func sameLink(got, want *Entry) bool {
	return got.Name == want.Name && got.Target == want.Target && got.C4ID == want.C4ID &&
		got.HardLink == want.HardLink && got.FlowDirection == want.FlowDirection &&
		got.FlowTarget == want.FlowTarget
}

func shortLabel(s string) string {
	if len(s) > 12 {
		return s[:12] + "…"
	}
	return s
}

func groupLabel(group int, id c4.ID) string {
	g := "ungrouped"
	if group > 0 {
		g = "group 2"
	}
	if id.IsNil() {
		return g + ", no C4 ID"
	}
	return g + ", C4 ID"
}
