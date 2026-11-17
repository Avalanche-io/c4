package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
)

// A store rooted at ~/.c4 whose config names it, split by a binary from
// before 2026-11-16: that split filed ~/.c4/config under co/, and from then
// on no store was configured. The config is read where it lies, never
// written or moved; with both files present the root's is read.
//
// The old split's move of the config is made here by a rename, as
// TestJournalReadsADisplacedJournal makes the journal's: this line's split
// no longer moves it (TestTreeStoreSplitMovesOnlyObjects). The same state
// made by the released 1.0.26 binary is measured in the PR.
func TestConfigDisplacedBySplitIsReadWhereItLies(t *testing.T) {
	home := t.TempDir()
	setenv(t, "HOME", home)
	setenv(t, "USERPROFILE", home)
	setenv(t, "C4_STORE", "")
	dotc4 := filepath.Join(home, ".c4")
	if err := os.MkdirAll(dotc4, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dotc4, "config")
	text := []byte("store = " + dotc4 + "\n") // the user's own config file, by hand
	if err := os.WriteFile(config, text, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewTreeStore(dotc4)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSplitThreshold(4)
	var ids []c4.ID
	for i := 0; i < 12; i++ {
		id, err := s.Put(bytes.NewReader([]byte(fmt.Sprint("object ", i))))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if info, err := os.Stat(filepath.Join(dotc4, "c4")); err != nil || !info.IsDir() {
		t.Fatalf("fixture: the root did not split (%v)", err)
	}
	// What the old split did to the config.
	displaced := filepath.Join(dotc4, "co", "config")
	if err := os.Mkdir(filepath.Dir(displaced), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(config, displaced); err != nil {
		t.Fatal(err)
	}

	got, err := OpenStore()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("no store configured: the displaced config was not read")
	}
	for _, id := range ids {
		f, err := got.Open(id)
		if err != nil {
			t.Fatalf("the configured store can't open %s: %v", id.String()[:12], err)
		}
		io.Copy(io.Discard, f)
		f.Close()
	}
	if b, err := os.ReadFile(displaced); err != nil || !bytes.Equal(b, text) {
		t.Fatalf("the displaced config was changed: %q, %v", b, err)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("a root config appeared: %v", err)
	}

	// Both present: the root's is read.
	other := filepath.Join(home, "other-store")
	if err := os.WriteFile(config, []byte("store = "+other+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := configuredPath(); p != other {
		t.Fatalf("with both configs present the store is %q, want the root config's %q", p, other)
	}
	if b, err := os.ReadFile(displaced); err != nil || !bytes.Equal(b, text) {
		t.Fatalf("the displaced config was changed: %q, %v", b, err)
	}
}
