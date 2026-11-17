package c4m

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// The journal's exclusive lock must exclude across a tree store's root
// split. One holder takes the lock on <store>/journal and keeps it; the
// store's root splits for real (TreeStore.Put past the threshold); then a
// second appender starts. It must wait until the holder lets go, and both
// claims must be in the journal afterwards.
//
// The holder is a second open of the file in this process, not a second
// process: the lock is per open file description (flock on Unix) or per
// handle (LockFileEx on Windows), so two opens contend as two processes
// would; the control at the start shows it.
//
// Through 2026-11-16 (master b82ec07) the split filed the journal under
// jo/: the holder's lock was on jo/journal, the appender's open created a
// new <store>/journal and its lock was granted at once, with the first
// still held (GC-REFUTATION-R2 F1 case a, F7). On Windows the split's
// rename of the open journal fails and the journal stays, so the old code
// fails this test on Unix only.
func TestJournalLockExcludesAcrossARootSplit(t *testing.T) {
	const wait = 500 * time.Millisecond
	root := t.TempDir()
	j := OpenJournal(root)
	at := func(i int) Claim {
		return Claim{ScanStart: time.Date(2026, 11, 16, 0, 0, i, 0, time.UTC), ID: c4.Identify(strings.NewReader(fmt.Sprint("lock claim ", i)))}
	}
	if err := j.Append(at(0)); err != nil {
		t.Fatal(err)
	}
	appendAsync := func(c Claim) chan error {
		done := make(chan error, 1)
		go func() { done <- OpenJournal(root).Append(c) }()
		return done
	}

	holder, err := os.OpenFile(j.Path(), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := lockFile(holder); err != nil {
		t.Fatal(err)
	}

	// The control: before any split, an appender waits for the holder.
	// The holder appends nothing; this appender's claim lands after the
	// release, below.
	control := appendAsync(at(1))
	select {
	case err := <-control:
		t.Fatalf("control: an append did not wait for the held lock (err %v); the lock does not exclude between two opens here", err)
	case <-time.After(wait):
	}

	// A real root split, by the library.
	s, err := store.NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSplitThreshold(16)
	s.SetSyncMode(store.SyncNone)
	for i := 0; i < 24; i++ {
		if _, err := s.Put(bytes.NewReader([]byte(fmt.Sprint("object ", i)))); err != nil {
			t.Fatal(err)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "c4")); err != nil || !info.IsDir() {
		t.Fatalf("fixture: the root did not split (%v)", err)
	}

	second := appendAsync(at(2))
	select {
	case err := <-second:
		t.Fatalf("after the root split an append did not wait for the held lock (err %v): it locked a different journal file", err)
	case err := <-control:
		t.Fatalf("after the root split the waiting append went through with the lock still held (err %v)", err)
	case <-time.After(wait):
	}

	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	for _, done := range []chan error{control, second} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("an append never finished after the lock was released")
		}
	}

	got, err := OpenJournal(root).Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != at(0).ID {
		t.Fatalf("journal holds %d claims after the split, want 3 starting with the first: %v", len(got), got)
	}
	seen := map[c4.ID]bool{}
	for _, c := range got[1:] {
		seen[c.ID] = true
	}
	if !seen[at(1).ID] || !seen[at(2).ID] {
		t.Fatalf("both waiting appenders' claims must be in the journal: %v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "jo", JournalName)); !os.IsNotExist(err) {
		t.Fatalf("the split displaced the journal to jo/ (%v)", err)
	}
}
