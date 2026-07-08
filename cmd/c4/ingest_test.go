package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// TestIDStoreBatchBarrierComplete verifies that the default
// batch-barrier ingest (`c4 id -s`) produces a complete, validating
// store: every file in the tree is retrievable by its C4 ID and the
// store holds no partial or temp objects.
func TestIDStoreBatchBarrierComplete(t *testing.T) {
	bin := buildC4(t)
	tree := t.TempDir()
	storeDir := filepath.Join(t.TempDir(), "store")

	// A tree with nested dirs and enough files to exercise the pool.
	var ids []c4.ID
	for d := 0; d < 4; d++ {
		dir := filepath.Join(tree, fmt.Sprintf("d%d", d))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for f := 0; f < 12; f++ {
			content := []byte(fmt.Sprintf("ingest content %d/%d\n", d, f))
			path := filepath.Join(dir, fmt.Sprintf("f%02d.txt", f))
			if err := os.WriteFile(path, content, 0644); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, c4.Identify(bytes.NewReader(content)))
		}
	}

	_, stderr, code := runC4WithEnv(t, bin,
		map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}

	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if !s.Has(id) {
			t.Fatalf("store missing %s after batch ingest", id)
		}
	}

	// Every stored object must validate: its name is the C4 ID of its
	// bytes. Walk skips non-ID files, so also check none are left.
	objects := 0
	err = s.Walk(func(id c4.ID, size int64) error {
		rc, err := s.Open(id)
		if err != nil {
			return err
		}
		defer rc.Close()
		if got := c4.Identify(rc); got != id {
			return fmt.Errorf("object %s does not validate (got %s)", id, got)
		}
		objects++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 48 files + 4 dir c4m objects + root dir c4m.
	if objects < len(ids) {
		t.Fatalf("store holds %d objects, want at least %d", objects, len(ids))
	}
	err = filepath.WalkDir(storeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return fmt.Errorf("leftover temp file: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestIDStoreDurableAndNoFsyncFlags verifies the explicit durability
// flags ingest identical content and reject being combined.
func TestIDStoreDurableAndNoFsyncFlags(t *testing.T) {
	bin := buildC4(t)
	tree := t.TempDir()
	content := []byte("flagged ingest content\n")
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}
	id := c4.Identify(bytes.NewReader(content))

	for _, flag := range []string{"--durable", "--no-fsync"} {
		storeDir := filepath.Join(t.TempDir(), "store")
		_, stderr, code := runC4WithEnv(t, bin,
			map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", flag, tree)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", flag, code, stderr)
		}
		s, err := store.NewTreeStore(storeDir)
		if err != nil {
			t.Fatal(err)
		}
		if !s.Has(id) {
			t.Fatalf("%s: store missing %s", flag, id)
		}
	}

	storeDir := filepath.Join(t.TempDir(), "store")
	_, _, code := runC4WithEnv(t, bin,
		map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", "--durable", "--no-fsync", tree)
	if code == 0 {
		t.Fatal("--durable --no-fsync should be rejected")
	}
}
