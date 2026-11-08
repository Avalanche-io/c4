package store

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// deadPid returns the pid of a process that has already exited: the
// test binary itself, run with a pattern no test matches.
func deadPid(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^NoSuchTest$")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return cmd.ProcessState.Pid()
}

func touch(t *testing.T, path string, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if age > 0 {
		old := time.Now().Add(-age)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSweepIngestOnOpen pins the boot rule: opening a store removes the
// incomplete ingests of dead writers on this host, and nothing else.
func TestSweepIngestOnOpen(t *testing.T) {
	root := t.TempDir()
	s, err := NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	// A real object, published: never touched.
	id, err := s.Put(bytes.NewReader([]byte("kept object\n")))
	if err != nil {
		t.Fatal(err)
	}

	dead := deadPid(t)
	live := os.Getpid()
	removeMe := []string{
		fmt.Sprintf(".ingest.%s.%d.111", ingestHost, dead), // dead writer, this host
		".ingest.222222", // pre-1.1 form, a day old
	}
	keepMe := []string{
		fmt.Sprintf(".ingest.%s.%d.333", ingestHost, live), // live writer
		fmt.Sprintf(".ingest.other-host.%d.444", dead),     // another host's pid space
		".ingest.555555", // pre-1.1 form, fresh
		fmt.Sprintf(".ingest.%s.%d.666.extra", ingestHost, 0), // parses as another host ("<host>.0"): kept
	}
	for _, n := range removeMe {
		touch(t, filepath.Join(root, n), 0)
	}
	old := filepath.Join(root, ".ingest.222222")
	t0 := time.Now().Add(-legacyIngestAge - time.Hour)
	if err := os.Chtimes(old, t0, t0); err != nil {
		t.Fatal(err)
	}
	for _, n := range keepMe {
		touch(t, filepath.Join(root, n), 0)
	}

	// Open again: the boot.
	s2, err := NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range removeMe {
		if _, err := os.Stat(filepath.Join(root, n)); !os.IsNotExist(err) {
			t.Errorf("%s: a dead writer's temp must be swept on open", n)
		}
	}
	for _, n := range keepMe {
		if _, err := os.Stat(filepath.Join(root, n)); err != nil {
			t.Errorf("%s: must be kept: %v", n, err)
		}
	}
	if !s2.Has(id) {
		t.Fatal("the sweep must never touch a store object")
	}
	// Nothing left to sweep.
	n, err := s2.SweepIngest()
	if err != nil || n != 0 {
		t.Fatalf("second sweep: %d removed, err %v", n, err)
	}
}

// TestSweepIngestKeepsOwnPending pins that a batch in progress survives a
// concurrent open of the same store: its temps belong to a live pid.
func TestSweepIngestKeepsOwnPending(t *testing.T) {
	root := t.TempDir()
	s, err := NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSyncMode(SyncBatch)
	id, err := s.Put(bytes.NewReader([]byte("pending object\n")))
	if err != nil {
		t.Fatal(err)
	}
	tmp := s.pending[id]
	if tmp == "" {
		t.Fatal("batch Put must leave the object pending")
	}
	if _, _, ok := parseIngestName(filepath.Base(tmp)); !ok {
		t.Fatalf("temp name %q must record host and pid", filepath.Base(tmp))
	}

	if _, err := NewTreeStore(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("a live writer's pending temp was swept: %v", err)
	}
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(id)); err != nil {
		t.Fatal("the pending object must publish after the barrier")
	}
}

func TestParseIngestName(t *testing.T) {
	cases := []struct {
		name string
		host string
		pid  int
		ok   bool
	}{
		{".ingest.abyss.local.4242.987654", "abyss.local", 4242, true},
		{".ingest.h.1.2", "h", 1, true},
		{".ingest.123456", "", 0, false},
		{".ingest.h.0.2", "", 0, false},
		{".ingest..5.2", "", 0, false},
		{"c4abc", "", 0, false},
	}
	for _, c := range cases {
		host, pid, ok := parseIngestName(c.name)
		if ok != c.ok || host != c.host || pid != c.pid {
			t.Errorf("%q: got (%q, %d, %v), want (%q, %d, %v)", c.name, host, pid, ok, c.host, c.pid, c.ok)
		}
	}
}
