package store

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestRamStore(t *testing.T) {
	rs := NewRAM()

	testdata := make(map[string]c4.ID)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("%06d", i)
		id := c4.Identify(strings.NewReader(key))
		testdata[key] = id

		w, err := rs.Create(id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(key)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// Verify all entries can be read back
	for k, v := range testdata {
		f, err := rs.Open(v)
		if err != nil {
			t.Error(err)
			continue
		}
		data, err := io.ReadAll(f)
		if err != nil {
			t.Error(err)
		}
		f.Close()
		if string(data) != k {
			t.Errorf("wrong data read, expected %q, got %q", k, string(data))
		}
	}

	// Duplicate Create should fail
	for _, v := range testdata {
		if _, err := rs.Create(v); err == nil {
			t.Error("duplicate Create should fail")
		}
		break // one check is enough
	}

	// Remove all and verify
	for _, v := range testdata {
		if err := rs.Remove(v); err != nil {
			t.Error(err)
		}
	}
	for _, v := range testdata {
		if _, err := rs.Open(v); err == nil {
			t.Error("Open should fail after Remove")
		}
		break
	}

	// Remove of non-existent should fail
	id := c4.Identify(strings.NewReader("nonexistent"))
	if err := rs.Remove(id); err == nil {
		t.Error("Remove of non-existent should fail")
	}
}
