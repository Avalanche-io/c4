package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// TestIDJournalsAMultiStore: C4_STORE naming two locations makes a
// MultiStore, which writes to its first; the claim is journaled beside
// what was written, and c4 log lists it. Through 2026-11-16 journalClaim
// returned silently for any store without Root(), so `c4 id -s` printed a
// claim it never journaled and `c4 log` found no journal (GC-DESIGN-R4
// S10).
func TestIDJournalsAMultiStore(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "p")
	writeTree(t, tree, map[string]string{"a.txt": "alpha"})
	home := filepath.Join(dir, "home")
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, d := range []string{home, a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"HOME": home, "USERPROFILE": home, "C4_STORE": a + "," + b}
	_, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("c4 id -s: exit %d: %s", code, stderr)
	}
	root := storedRoot(stderr)
	if root == "" {
		t.Fatalf("no stored: line: %s", stderr)
	}
	if strings.Contains(stderr, "not journaled") {
		t.Errorf("a MultiStore with a local write store says the claim is not journaled: %s", stderr)
	}
	log, lstderr, code := runC4WithEnv(t, bin, env, "log")
	if code != 0 || !strings.Contains(log, root) {
		t.Errorf("c4 log (exit %d) does not list the claim %s:\n%s%s", code, root, log, lstderr)
	}
}

// TestJournalClaimWithoutALocalStore: a store with no local root (a remote
// store) has no journal. The claim is stored, and journalClaim says once on
// stderr that it is not journaled, rather than nothing; the run goes on
// (decided 2026-11-16: collection is refused on stores that are not local,
// so an unjournaled remote claim cannot cost content). Through 2026-11-16
// it returned silently.
func TestJournalClaimWithoutALocalStore(t *testing.T) {
	id := c4.Identify(strings.NewReader("a claim"))
	stderr := captureStderr(t, func() { journalClaim(store.NewRAM(), id, time.Now()) })
	if !strings.Contains(stderr, id.String()) || !strings.Contains(stderr, "not journaled") {
		t.Fatalf("journalClaim on a store with no local root wrote %q; want it to say %s is not journaled", stderr, id)
	}
	if n := strings.Count(stderr, "not journaled"); n != 1 {
		t.Errorf("said %d times, want once: %q", n, stderr)
	}
}

// captureStderr runs f with os.Stderr sent to a file and returns what it
// wrote.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	saved := os.Stderr
	os.Stderr = out
	defer func() { os.Stderr = saved }()
	f()
	os.Stderr = saved
	b, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestMultiStoreRefusesAnUnknownJournalVersionBeforeIngest: the journal a
// MultiStore's claims go to (its write store's) is version-checked before
// anything is ingested, as a single store's is (c4 issue #44). Without
// that, -s into a MultiStore whose write store holds a newer-format journal
// stored its objects first and only then met the journal.
func TestMultiStoreRefusesAnUnknownJournalVersionBeforeIngest(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir, journal := storeWithUnknownJournal(t, bin, dir)
	before := storeObjects(t, storeDir)
	other := filepath.Join(dir, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, "second")
	writeTree(t, second, map[string]string{"b.txt": "bravo, new content"})
	env := map[string]string{"C4_STORE": storeDir + "," + other}
	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", second)
	if code == 0 {
		t.Errorf("c4 id -s exited 0 into a MultiStore whose write store has a version-2 journal, printing %q", stdout)
	}
	if !strings.Contains(stderr, `"2"`) {
		t.Errorf("stderr doesn't name the version it found:\n%s", stderr)
	}
	after, err := os.ReadFile(filepath.Join(storeDir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, journal) {
		t.Errorf("the version-2 journal was changed:\nbefore %q\nafter  %q", journal, after)
	}
	if got := storeObjects(t, storeDir); strings.Join(got, "\n") != strings.Join(before, "\n") {
		t.Errorf("objects were stored before the journal was refused:\nbefore %v\nafter  %v", before, got)
	}
}
