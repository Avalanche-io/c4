package c4m

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// A description that names an external base keeps it through encode and
// decode. Through 2026-11-13 the encoder dropped Base, so Unmarshal then
// Marshal turned a block on a base into a standalone description of the
// block's own entries (found porting the chain rules to c4-swift, which
// copied the encoder).
func TestEncodeKeepsExternalBase(t *testing.T) {
	base := c4.Identify(strings.NewReader("prior block"))
	entry := "-rw-r--r-- 2026-01-01T00:00:00Z 1 a.txt " + c4.Identify(strings.NewReader("a")).String() + "\n"
	text := base.String() + "\n" + entry

	m, err := Unmarshal([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if m.Base != base {
		t.Fatalf("decode: base %s, want %s", m.Base, base)
	}
	out, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != text {
		t.Fatalf("encode must write the base first and change nothing else:\n got %q\nwant %q", out, text)
	}
	back, err := Unmarshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if back.Base != base || len(back.Entries) != 1 {
		t.Fatalf("round trip: base %s with %d entries, want %s with 1", back.Base, len(back.Entries), base)
	}

	// Pretty output names the base too.
	pretty, err := MarshalPretty(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(pretty), base.String()+"\n") {
		t.Fatalf("pretty encode must start with the base:\n%s", pretty)
	}

	// A manifest with no base is encoded exactly as before: no bare line.
	m.Base = c4.ID{}
	plain, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != entry {
		t.Fatalf("no base, no bare line: got %q", plain)
	}
}

// EncodePatch's first line is its OldID; a base the patch manifest carries
// is not written a second time.
func TestEncodePatchWritesOneFirstLine(t *testing.T) {
	old := c4.Identify(strings.NewReader("old state"))
	patch, err := Unmarshal([]byte(c4.Identify(strings.NewReader("other")).String() + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 1 a.txt " + c4.Identify(strings.NewReader("a")).String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := NewEncoder(&buf).EncodePatch(&PatchResult{Patch: patch, OldID: old}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || lines[0] != old.String() {
		t.Fatalf("want the OldID line then the entry, got:\n%s", buf.String())
	}
	if patch.Base.IsNil() {
		t.Fatal("EncodePatch must not modify the caller's manifest")
	}
}
