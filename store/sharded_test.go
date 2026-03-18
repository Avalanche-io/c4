package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestShardedFolder(t *testing.T) {
	dir := t.TempDir()
	sf := ShardedFolder(dir)

	// Create and read back 100 objects
	testdata := make(map[string]c4.ID)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("%06d", i)
		id := c4.Identify(strings.NewReader(key))
		testdata[key] = id

		w, err := sf.Create(id)
		if err != nil {
			t.Fatalf("Create %s: %v", key, err)
		}
		w.Write([]byte(key))
		w.Close()
	}

	// Verify sharded layout: files should be in subdirectories
	for key, id := range testdata {
		idStr := id.String()
		shard := idStr[3:5]
		shardedPath := filepath.Join(dir, shard, idStr)
		if _, err := os.Stat(shardedPath); err != nil {
			t.Errorf("sharded file missing for %s: %v", key, err)
		}

		// Path method should return the sharded path
		if sf.Path(id) != shardedPath {
			t.Errorf("Path mismatch for %s", key)
		}
	}

	// Verify Has and Open
	for key, id := range testdata {
		if !sf.Has(id) {
			t.Errorf("Has returned false for %s", key)
		}

		rc, err := sf.Open(id)
		if err != nil {
			t.Errorf("Open %s: %v", key, err)
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if string(data) != key {
			t.Errorf("content mismatch for %s: got %q", key, data)
		}
	}

	// Duplicate Create should fail
	id0 := testdata["000000"]
	if _, err := sf.Create(id0); err == nil {
		t.Error("duplicate Create should fail")
	}

	// Remove and verify
	if err := sf.Remove(id0); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if sf.Has(id0) {
		t.Error("Has returned true after Remove")
	}
}

func TestShardedFolderFlatFallback(t *testing.T) {
	dir := t.TempDir()
	sf := ShardedFolder(dir)

	// Create a file in flat layout (simulating pre-sharding store)
	data := "flat content"
	id := c4.Identify(strings.NewReader(data))
	flatPath := filepath.Join(dir, id.String())
	os.WriteFile(flatPath, []byte(data), 0644)

	// Should find via Has
	if !sf.Has(id) {
		t.Error("Has returned false for flat-layout file")
	}

	// Should read via Open
	rc, err := sf.Open(id)
	if err != nil {
		t.Fatalf("Open flat: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != data {
		t.Errorf("flat content mismatch: got %q", got)
	}

	// Create should detect flat duplicate
	if _, err := sf.Create(id); err == nil {
		t.Error("Create should fail when flat-layout file exists")
	}

	// Remove should work on flat file
	if err := sf.Remove(id); err != nil {
		t.Fatalf("Remove flat: %v", err)
	}
	if sf.Has(id) {
		t.Error("Has returned true after Remove")
	}
}
