package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestRamStore(t *testing.T) {

	rs := NewRAM()

	testdata := make(map[string]c4.ID)
	for i := 0; i < 100; i++ {

		// Create arbitrary test data
		key := fmt.Sprintf("%06d", i)

		// Create c4 id of the test data
		id := c4.Identify(strings.NewReader(key))
		testdata[key] = id

		// Test Ram store `Create` method
		w, err := rs.Create(id)
		if err != nil {
			t.Fatal(err)
		}

		// Write data to the Ram store
		_, err = w.Write([]byte(key))
		if err != nil {
			t.Fatal(err)
		}

		// Close the Ram store
		err = w.Close()
		if err != nil {
			t.Fatal(err)
		}

	}

	// Check that Ram map is acutally populated with c4 ids.
	rsmap := map[c4.ID][]byte(*rs)

	var ids []string
	for _, v := range testdata {
		ids = append(ids, v.String())
	}

	if len(rsmap) != len(testdata) {
		t.Errorf("wrong number of results got %d expected %d", len(rsmap), len(testdata))
	}

	// Test all filenames against all ids
	for id1, data1 := range rsmap {
		found := false
		for _, id2 := range ids {
			if id1.String() == id2 && testdata[string(data1)] == id1 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("find file name that does not match an id in the list %q", id1)
		}
	}

	// Test all ids against all filenames
	for _, id1 := range ids {
		found := false
		for id2, _ := range rsmap {
			if id1 == id2.String() {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("an id was not matched by a file %s", id1)
		}
	}

	// Test Ram store `Open` method
	for k, v := range testdata {

		f, err := rs.Open(v)
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

	// Test Ram remove
	i := len(testdata)
	for _, v := range testdata {
		err := rs.Remove(v)
		if err != nil {
			t.Error(err)
		}
		i--
		if len(rsmap) != i {
			t.Errorf("key not removed from map")
		}
	}

}
