package c4m

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// A journal names its format version in its header (@c4 journal 1). A
// reader refuses a version it doesn't know rather than reading it as its
// own, and never appends to it (c4 issue #44; GC-DESIGN-R4 §7 S11).
//
// The fixtures are written by the library's own journal writer. The test
// then changes the header's version digit from 1 to 2, and nothing else:
// a journal of a version this package doesn't know is exactly the input
// under test, and no writer in this package can produce one.

// unknownVersionJournal writes a two-claim journal under root with Append,
// changes its version digit to 2, and returns the file's bytes.
func unknownVersionJournal(t *testing.T, root string) []byte {
	t.Helper()
	j := OpenJournal(root)
	for _, c := range []Claim{testClaim("one", 10), testClaim("two", 11)} {
		if err := j.Append(c); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("@c4 journal 1\n")) {
		t.Fatalf("the writer's header is not @c4 journal 1: %q", data[:20])
	}
	data[len("@c4 journal ")] = '2' // the one deliberate byte
	if err := os.WriteFile(j.Path(), data, 0644); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecodeJournalRefusesAnUnknownVersion(t *testing.T) {
	root := t.TempDir()
	data := unknownVersionJournal(t, root)

	if !IsJournal(data) {
		t.Error("IsJournal does not recognise a version-2 journal as a journal")
	}
	claims, err := DecodeJournal(data)
	if err == nil {
		t.Fatalf("DecodeJournal read a version-2 journal as version 1: %d claims", len(claims))
	}
	if msg := err.Error(); !strings.Contains(msg, `"2"`) || !strings.Contains(msg, "version 1") {
		t.Errorf("the error doesn't name the version found and the version known: %v", err)
	}
	if _, err := OpenJournal(root).Claims(); err == nil {
		t.Error("Journal.Claims read a version-2 journal")
	}
}

func TestAppendRefusesAnUnknownVersion(t *testing.T) {
	root := t.TempDir()
	data := unknownVersionJournal(t, root)
	// A torn tail too: the version must be refused before the tail is
	// truncated, so the file is left byte for byte as it was.
	data = append(data, "2026-07-13T12:00"...)
	j := OpenJournal(root)
	if err := os.WriteFile(j.Path(), data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := j.Append(testClaim("three", 12)); err == nil {
		t.Error("Append wrote to a version-2 journal")
	}
	after, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, data) {
		t.Errorf("Append changed a version-2 journal:\nbefore %q\nafter  %q", data, after)
	}
}

func TestJournalVersionOneIsUnchanged(t *testing.T) {
	root := t.TempDir()
	j := OpenJournal(root)
	want := []Claim{testClaim("one", 10), testClaim("two", 11)}
	for _, c := range want {
		if err := j.Append(c); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("@c4 journal 1\n")) {
		t.Fatalf("header is not @c4 journal 1: %q", data)
	}
	got, err := DecodeJournal(data)
	if err != nil || len(got) != len(want) {
		t.Fatalf("version 1: %d claims, %v; want %d", len(got), err, len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || !got[i].ScanStart.Equal(want[i].ScanStart) {
			t.Errorf("claim %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if claims, err := DecodeJournal([]byte("@c4 journal 1\n")); err != nil || len(claims) != 0 {
		t.Errorf("header-only version 1: %d claims, %v; want none, no error", len(claims), err)
	}
}
