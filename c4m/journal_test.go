package c4m

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func testClaim(content string, hour int) Claim {
	return Claim{
		ScanStart: time.Date(2026, 7, 13, hour, 0, 0, 0, time.UTC),
		ID:        c4.Identify(strings.NewReader(content)),
	}
}

// TestJournalRoundTrip pins the append/read cycle: both fields
// survive, in append order, under the @c4 journal magic header —
// and the file is definitively NOT c4m (every conforming c4m parser
// rejects @-lines).
func TestJournalRoundTrip(t *testing.T) {
	root := t.TempDir()
	j := OpenJournal(root)

	claims := []Claim{
		testClaim("first snapshot", 10),
		testClaim("second snapshot", 11),
		testClaim("blob!", 12),
	}
	for _, c := range claims {
		if err := j.Append(c); err != nil {
			t.Fatal(err)
		}
	}

	got, err := j.Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(claims) {
		t.Fatalf("expected %d claims, got %d", len(claims), len(got))
	}
	for i, want := range claims {
		g := got[i]
		if !g.ScanStart.Equal(want.ScanStart) || g.ID != want.ID {
			t.Fatalf("claim %d mismatch:\n got %+v\nwant %+v", i, g, want)
		}
	}

	data, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	// Exactly one magic header, then one line per claim.
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if lines[0] != "@c4 journal 1" {
		t.Fatalf("missing magic header, got %q", lines[0])
	}
	if len(lines) != len(claims)+1 {
		t.Fatalf("expected %d lines, got %d", len(claims)+1, len(lines))
	}
	// Two fields per claim line, ID last (the awk $NF contract).
	for _, l := range lines[1:] {
		f := strings.Fields(l)
		if len(f) != 2 {
			t.Fatalf("claim line must have 2 fields: %q", l)
		}
		if _, err := c4.Parse(f[1]); err != nil {
			t.Fatalf("last field must be a C4 ID: %q", l)
		}
	}

	// The journal is NOT c4m: the standard c4m decoder must reject it.
	if _, err := NewDecoder(strings.NewReader(string(data))).Decode(); err == nil {
		t.Fatal("c4m decoder accepted a journal — the @ magic must be rejected")
	}
	if !IsJournal(data) {
		t.Fatal("IsJournal must recognize the magic")
	}
}

// TestJournalTornTail pins crash recovery: a partial final line (a
// writer that died mid-append) is invisible to readers and removed by
// the next append.
func TestJournalTornTail(t *testing.T) {
	root := t.TempDir()
	j := OpenJournal(root)

	first := testClaim("content a", 10)
	if err := j.Append(first); err != nil {
		t.Fatal(err)
	}

	// Simulate a torn append: partial line, no trailing newline.
	f, err := os.OpenFile(j.Path(), os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("2026-07-13T10:30:00Z c4torn"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Readers ignore the torn tail.
	got, err := j.Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != first.ID {
		t.Fatalf("torn tail visible to reader: %+v", got)
	}

	// The next append truncates it and lands cleanly.
	second := testClaim("content b", 11)
	if err := j.Append(second); err != nil {
		t.Fatal(err)
	}
	got, err = j.Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].ID != second.ID {
		t.Fatalf("expected clean 2-claim journal after torn-tail append, got %+v", got)
	}
	data, _ := os.ReadFile(j.Path())
	if strings.Contains(string(data), "c4torn") {
		t.Fatal("torn line survived the truncating append")
	}
}

// TestJournalConcurrentAppends pins the lock: concurrent appenders
// serialize; every claim lands exactly once, no interleaved bytes,
// and exactly one magic header.
func TestJournalConcurrentAppends(t *testing.T) {
	root := t.TempDir()
	const n = 24

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			j := OpenJournal(root) // separate fds, like separate processes
			c := testClaim(strings.Repeat("x", i+1), 10)
			if err := j.Append(c); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	got, err := OpenJournal(root).Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("expected %d claims, got %d", n, len(got))
	}
	seen := make(map[c4.ID]bool)
	for _, g := range got {
		if seen[g.ID] {
			t.Fatalf("duplicate claim %s", g.ID)
		}
		seen[g.ID] = true
	}
	data, _ := os.ReadFile(OpenJournal(root).Path())
	if strings.Count(string(data), "@c4 journal") != 1 {
		t.Fatal("expected exactly one magic header under concurrent appends")
	}
}

// TestJournalEmptyAndMissing pins the null cases.
func TestJournalEmptyAndMissing(t *testing.T) {
	j := OpenJournal(t.TempDir())
	got, err := j.Claims()
	if err != nil || got != nil {
		t.Fatalf("missing journal should read as empty, got %v, %v", got, err)
	}
	if err := os.WriteFile(j.Path(), nil, 0644); err != nil {
		t.Fatal(err)
	}
	got, err = j.Claims()
	if err != nil || len(got) != 0 {
		t.Fatalf("empty journal should read as empty, got %v, %v", got, err)
	}
}

// TestJournalRejectsHeaderlessData pins the misinterpretation guard in
// the other direction: claim-shaped data without the magic is refused.
func TestJournalRejectsHeaderlessData(t *testing.T) {
	j := OpenJournal(t.TempDir())
	line := testClaim("x", 10).Line() + "\n"
	if err := os.WriteFile(j.Path(), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Claims(); err == nil {
		t.Fatal("headerless journal data must be refused")
	}
}

// A journal a tree store's root split displaced to jo/journal (every
// version through 2026-11-16 filed every root file by its first two
// characters) is read where it lies, before the root's, so the history
// stays whole and in order. The displaced file is never written.
func TestJournalReadsADisplacedJournal(t *testing.T) {
	root := t.TempDir()
	j := OpenJournal(root)
	at := func(i int) Claim {
		return Claim{ScanStart: time.Date(2026, 11, 1, 0, 0, i, 0, time.UTC), ID: c4.Identify(strings.NewReader(fmt.Sprint("claim ", i)))}
	}
	for i := 0; i < 3; i++ {
		if err := j.Append(at(i)); err != nil {
			t.Fatal(err)
		}
	}
	// What the split did: rename the root's journal into jo/.
	if err := os.Mkdir(filepath.Join(root, "jo"), 0o755); err != nil {
		t.Fatal(err)
	}
	displaced := filepath.Join(root, "jo", JournalName)
	if err := os.Rename(j.Path(), displaced); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(displaced)
	if err != nil {
		t.Fatal(err)
	}
	for i := 3; i < 5; i++ { // appends after the split start a root journal
		if err := j.Append(at(i)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := OpenJournal(root).Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("Claims: %d claims, want 5 (3 displaced, 2 at the root)", len(got))
	}
	for i, c := range got {
		if c.ID != at(i).ID {
			t.Errorf("claim %d is %s, want %s: out of order", i+1, c.Line(), at(i).Line())
		}
	}
	after, err := os.ReadFile(displaced)
	if err != nil || !bytes.Equal(before, after) {
		t.Errorf("the displaced journal was written: %v", err)
	}
	// A file at jo/journal that is not a journal is not read as one.
	if err := os.WriteFile(displaced, []byte("not a journal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := OpenJournal(root).Claims(); err != nil || len(got) != 2 {
		t.Errorf("with a non-journal at jo/journal: %d claims, %v; want the root's 2", len(got), err)
	}
}
