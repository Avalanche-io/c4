package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestMapStore(t *testing.T) {
	tmp, done, err := MkTmp("TestMapStore")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	m := make(map[c4.ID]string)
	ms := NewMap(m)
	var ids []c4.ID
	for i := 100; i > 0; i-- {
		data := fmt.Sprintf("%04d", i)
		filename := filepath.Join(tmp, data)
		f, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(data)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		id := c4.Identify(bytes.NewReader([]byte(data)))
		ids = append(ids, id)
		ms.LoadOrStore(id, filename)
	}

	ms.Range(func(id c4.ID, path string) bool {
		if m[id] != path {
			t.Error("wrong map content")
		}
		return true
	})

	if len(m) != len(ids) {
		t.Errorf("counts don't match %d %d", len(m), len(ids))
	}

	// Test all filenames against all ids
	for i, id := range ids {
		f, err := ms.Open(id)
		if err != nil {
			t.Fatal(err)
		}
		testid := c4.Identify(f)
		f.Close()
		if testid != id {
			t.Fatalf("wrong id content %d", i)
		}

	}
}

func MkTmp(name string) (string, func(), error) {
	path := os.TempDir()
	path = filepath.Join(path, name)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return "", func() {}, err
	}

	return path, func() {
		os.RemoveAll(path)
	}, nil
}
