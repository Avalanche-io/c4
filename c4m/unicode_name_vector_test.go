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

// The shared unicode-name vector (c4m/testdata/unicode-name-vector): names,
// a symlink target and a flow-target path that are not ASCII, written by
// the library. Every reader must decode canonical.c4m and pretty.c4m to
// exactly the names listed in names.txt and to root-id.txt. Through
// 1.0.25 c4m-swift refused every line whose name was not ASCII: its field
// parsers walked UTF-8 bytes but sliced the line by Character offsets, so
// the C4 ID field lost one position per extra byte (measured 2026-11-11).

const (
	uniNFC   = "café.txt"
	uniNFD   = "café.txt"
	uniCJK   = "日本.txt"
	uniEmoji = "\U0001F600.txt"
	uniDir   = "Ünïcödé/"
	uniChild = "naïve.txt"
	uniLink  = "链接"
	uniTgt   = "目标.txt"
	uniFlow  = "发送/"
	uniFlowT = "nas:データ/"
)

func unicodeNameManifest() *Manifest {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id := func(s string) c4.ID { return c4.Identify(strings.NewReader(s)) }
	m := NewManifest()
	for _, n := range []string{uniNFC, uniNFD, uniCJK, uniEmoji} {
		m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: int64(len(n)), Name: n, C4ID: id(n)})
	}
	m.AddEntry(&Entry{Mode: os.ModeSymlink | 0o777, Timestamp: ts, Size: int64(len(uniTgt)), Name: uniLink, Target: uniTgt})
	m.AddEntry(&Entry{Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1, Name: uniDir})
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: int64(len(uniChild)), Name: uniChild, Depth: 1, C4ID: id(uniChild)})
	m.AddEntry(&Entry{Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1, Name: uniFlow, FlowDirection: FlowOutbound, FlowTarget: uniFlowT})
	m.SortEntries()
	Normalize(m)
	return m
}

func TestUnicodeNameVector(t *testing.T) {
	dir := filepath.Join("testdata", "unicode-name-vector")
	m := unicodeNameManifest()
	canonical, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var pretty bytes.Buffer
	if err := NewEncoder(&pretty).SetPretty(true).Encode(m); err != nil {
		t.Fatal(err)
	}
	var names strings.Builder
	for _, e := range m.Entries {
		names.WriteString(strings.Repeat("  ", e.Depth) + e.Name + "\n")
	}
	// A chain whose base holds café.txt in NFC and whose patch adds the NFD
	// spelling: two names, so the result holds both (a reader keying names
	// by canonically equivalent text merges them into one).
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nfcOnly := NewManifest()
	nfcOnly.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 3, Name: uniNFC, C4ID: c4.Identify(strings.NewReader("one"))})
	both := nfcOnly.Copy()
	both.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 3, Name: uniNFD, C4ID: c4.Identify(strings.NewReader("two"))})
	both.SortEntries()
	baseText, err := Marshal(nfcOnly)
	if err != nil {
		t.Fatal(err)
	}
	var chain bytes.Buffer
	chain.Write(baseText)
	if err := NewEncoder(&chain).EncodePatch(PatchDiff(nfcOnly, both)); err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"canonical.c4m":             canonical,
		"pretty.c4m":                pretty.Bytes(),
		"names.txt":                 []byte(names.String()),
		"root-id.txt":               []byte(m.ComputeC4ID().String() + "\n"),
		"nfc-nfd-chain.c4m":         chain.Bytes(),
		"nfc-nfd-chain-root-id.txt": []byte(both.ComputeC4ID().String() + "\n"),
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

	got, err := Unmarshal(files["nfc-nfd-chain.c4m"])
	if err != nil || len(got.Entries) != 2 || got.ComputeC4ID() != both.ComputeC4ID() {
		t.Fatalf("nfc-nfd-chain.c4m must resolve to both spellings: %v %v", got, err)
	}

	// A conforming reader: both forms decode to exactly these names, the
	// link's target, the flow target, and the pinned root ID.
	for _, form := range []string{"canonical.c4m", "pretty.c4m"} {
		got, err := Unmarshal(files[form])
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if got.ComputeC4ID() != m.ComputeC4ID() {
			t.Errorf("%s: root ID %s, want %s", form, got.ComputeC4ID(), m.ComputeC4ID())
		}
		byName := map[string]*Entry{}
		for _, e := range got.Entries {
			byName[e.Name] = e
		}
		for _, n := range []string{uniNFC, uniNFD, uniCJK, uniEmoji, uniDir, uniChild, uniLink, uniFlow} {
			if byName[n] == nil {
				t.Errorf("%s: no entry named %q", form, n)
			}
		}
		if e := byName[uniLink]; e == nil || e.Target != uniTgt {
			t.Errorf("%s: the link's target must be %q", form, uniTgt)
		}
		if e := byName[uniFlow]; e == nil || e.FlowTarget != uniFlowT {
			t.Errorf("%s: the flow target must be %q", form, uniFlowT)
		}
	}
}
