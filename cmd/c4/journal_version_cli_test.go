package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Every c4 verb that reads the store's journal, or stores and claims into
// it, refuses a journal of a format version it doesn't know and exits
// non-zero. It never misreads the journal, never stores anything into its
// store, and never appends to it (c4 issue #44).
//
// The journal is written by c4 itself (c4 id -s). The test then changes
// the header's version digit from 1 to 2, and nothing else: an unknown
// version is exactly the case under test.

// storeWithUnknownJournal makes a store holding one snapshot, changes its
// journal's version digit to 2, and returns the store and the journal's
// bytes.
func storeWithUnknownJournal(t *testing.T, bin, dir string) (string, []byte) {
	t.Helper()
	storeDir := filepath.Join(dir, "store")
	first := filepath.Join(dir, "first")
	writeTree(t, first, map[string]string{"a.txt": "alpha"})
	if _, stderr, code := runC4WithEnv(t, bin, map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", first); code != 0 {
		t.Fatalf("setup snapshot: exit %d: %s", code, stderr)
	}
	path := filepath.Join(storeDir, "journal")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("@c4 journal 1\n")) {
		t.Fatalf("c4 wrote a journal header other than @c4 journal 1: %q", data)
	}
	data[len("@c4 journal ")] = '2' // the one deliberate byte
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return storeDir, data
}

// storeObjects lists the store's files other than its journal.
func storeObjects(t *testing.T, storeDir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(storeDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() != "journal" {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func TestLogRefusesAnUnknownJournalVersion(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir, _ := storeWithUnknownJournal(t, bin, dir)

	for _, args := range [][]string{{"log"}, {"log", filepath.Join(storeDir, "journal")}} {
		stdout, stderr, code := runC4WithEnv(t, bin, map[string]string{"C4_STORE": storeDir}, args...)
		if code == 0 {
			t.Errorf("c4 %s exited 0 on a version-2 journal, printing:\n%s", strings.Join(args, " "), stdout)
		}
		if !strings.Contains(stderr, `"2"`) {
			t.Errorf("c4 %s: stderr doesn't name the version it found:\n%s", strings.Join(args, " "), stderr)
		}
	}
}

func TestStoreRefusesAnUnknownJournalVersionBeforeIngest(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir, journal := storeWithUnknownJournal(t, bin, dir)
	before := storeObjects(t, storeDir)

	second := filepath.Join(dir, "second")
	writeTree(t, second, map[string]string{"b.txt": "bravo, new content"})
	stdout, stderr, code := runC4WithEnv(t, bin, map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", second)
	if code == 0 {
		t.Errorf("c4 id -s exited 0 into a store with a version-2 journal, printing %q", stdout)
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

func TestWarmRescanRefusesAnUnknownJournalVersion(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "alpha"})
	env := map[string]string{"C4_STORE": storeDir}
	guide, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("setup snapshot: exit %d: %s", code, stderr)
	}
	guidePath := filepath.Join(dir, "guide.c4m")
	os.WriteFile(guidePath, []byte(guide), 0644)
	path := filepath.Join(storeDir, "journal")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len("@c4 journal ")] = '2' // the one deliberate byte
	os.WriteFile(path, data, 0644)

	// -c finds the guide's scan start in the journal. A journal it can't
	// read must be refused, not read as having no claim for the guide.
	if _, stderr, code := runC4WithEnv(t, bin, env, "id", "-c", guidePath, tree); code == 0 {
		t.Errorf("c4 id -c exited 0 with a version-2 journal:\n%s", stderr)
	}
}
