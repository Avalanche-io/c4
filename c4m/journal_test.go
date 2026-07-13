package c4m

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func testClaim(name, content string) Claim {
	return Claim{
		ScanStart: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		Size:      int64(len(content)),
		Name:      name,
		Origin:    "abyss:/Users/joshua/proj",
		ID:        c4.Identify(strings.NewReader(content)),
	}
}

// TestJournalRoundTrip pins the append/read cycle: every field
// survives, in order, and the file parses as an ordinary patch chain.
func TestJournalRoundTrip(t *testing.T) {
	root := t.TempDir()
	j := OpenJournal(root)

	claims := []Claim{
		testClaim("proj.c4m", "first snapshot"),
		testClaim("proj.c4m", "second snapshot"),
		{ScanStart: time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC),
			Size: 5, Name: "stdin", ID: c4.Identify(strings.NewReader("blob!"))},
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
		if !g.ScanStart.Equal(want.ScanStart) || g.Size != want.Size ||
			g.Name != want.Name || g.Origin != want.Origin || g.ID != want.ID {
			t.Fatalf("claim %d mismatch:\n got %+v\nwant %+v", i, g, want)
		}
	}

	// The journal is an ordinary c4m patch chain: standard decode works.
	data, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	sections, err := DecodePatchChain(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("journal is not a valid patch chain: %v", err)
	}
	if len(sections) != len(claims) {
		t.Fatalf("expected %d sections, got %d", len(claims), len(sections))
	}
}

// TestJournalTornTail pins crash recovery: a partial final line (a
// writer that died mid-append) is invisible to readers and removed by
// the next append.
func TestJournalTornTail(t *testing.T) {
	root := t.TempDir()
	j := OpenJournal(root)

	first := testClaim("a.c4m", "content a")
	if err := j.Append(first); err != nil {
		t.Fatal(err)
	}

	// Simulate a torn append: partial line, no trailing newline.
	f, err := os.OpenFile(j.Path(), os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("- 2026-07-13T10:30:00Z 99 torn.c4m c4partial"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Readers ignore the torn tail.
	got, err := j.Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "a.c4m" {
		t.Fatalf("torn tail visible to reader: %+v", got)
	}

	// The next append truncates it and lands cleanly.
	second := testClaim("b.c4m", "content b")
	if err := j.Append(second); err != nil {
		t.Fatal(err)
	}
	got, err = j.Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Name != "b.c4m" {
		t.Fatalf("expected clean 2-claim journal after torn-tail append, got %+v", got)
	}
	data, _ := os.ReadFile(j.Path())
	if strings.Contains(string(data), "torn.c4m") {
		t.Fatal("torn line survived the truncating append")
	}
}

// TestJournalConcurrentAppends pins the lock: concurrent appenders
// serialize; every claim lands exactly once, no interleaved bytes.
func TestJournalConcurrentAppends(t *testing.T) {
	root := t.TempDir()
	const n = 24

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			j := OpenJournal(root) // separate fds, like separate processes
			c := testClaim("proj.c4m", strings.Repeat("x", i+1))
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
