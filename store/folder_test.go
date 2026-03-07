package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestFolderStore(t *testing.T) {
	path := os.TempDir()

	path = filepath.Join(path, "folder_test")
	err := os.Mkdir(path, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(path)

	folderStore := Folder(path)

	testdata := make(map[string]c4.ID)
	for i := 0; i < 100; i++ {

		// Create arbitrary test data
		key := fmt.Sprintf("%06d", i)

		// Create c4 id of the test data
		id := c4.Identify(strings.NewReader(key))
		testdata[key] = id

		// Test Folder store `Create` method
		w, err := folderStore.Create(id)
		if err != nil {
			t.Fatal(err)
		}

		// Write data to the Folder store
		_, err = w.Write([]byte(key))
		if err != nil {
			t.Fatal(err)
		}

		// Close the Folder store
		err = w.Close()
		if err != nil {
			t.Fatal(err)
		}

	}

	// Check that files with the appropreate C4 id are indeed located in the test
	// folder.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, v := range testdata {
		ids = append(ids, v.String())
	}

	if len(names) != len(ids) {
		t.Errorf("wrong number of results got %d expected %d", len(names), len(ids))
	}

	// Test all filenames against all ids
	for _, name := range names {
		found := false
		for _, id := range ids {
			if name == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("find file name that does not match an id in the list %q", name)
		}
	}

	// Test all ids against all filenames
	for _, id := range ids {
		found := false
		for _, name := range names {
			if id == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("an id was not matched by a file %s", id)
		}
	}

	// Test Folder store `Open` method
	for k, v := range testdata {

		f, err := folderStore.Open(v)
		if err != nil {
			t.Error(err)
		}

		data := make([]byte, 512)
		n, err := f.Read(data)
		if err != nil {
			t.Error(err)
		}

		data = data[:n]
		if string(data) != k {
			t.Errorf("wrong data read from file, expted %q, go %q", k, string(data))
		}
	}

}

func TestDurableWrite(t *testing.T) {
	dir := t.TempDir()
	fs := Folder(dir)

	key := "durable-test-data"
	id := c4.Identify(strings.NewReader(key))

	w, err := fs.Create(id)
	if err != nil {
		t.Fatal(err)
	}

	// Final file should not exist before Close
	finalPath := filepath.Join(dir, id.String())
	if _, err := os.Stat(finalPath); err == nil {
		t.Fatal("final file exists before Close")
	}

	if _, err := w.Write([]byte(key)); err != nil {
		t.Fatal(err)
	}

	// Still should not exist
	if _, err := os.Stat(finalPath); err == nil {
		t.Fatal("final file exists before Close")
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Now it should exist
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatal("final file missing after Close")
	}

	// No temp files should remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp.") {
			t.Errorf("temp file not cleaned up: %s", e.Name())
		}
	}

	// Verify content
	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != key {
		t.Errorf("wrong content: got %q, want %q", string(data), key)
	}
}
