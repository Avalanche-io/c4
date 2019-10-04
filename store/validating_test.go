package store

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestValidatingStore(t *testing.T) {
	var st Store
	ramst := NewRAM()
	st = NewValidating(ramst)

	rand.Seed(time.Now().UnixNano())
	randIndx := int(rand.Int63n(100))
	t.Logf("random index: %d", randIndx)
	testdata := make(map[string]c4.ID)
	for i := 0; i < 100; i++ {

		// Create arbitrary test data
		key := fmt.Sprintf("%06d", i)

		// Create c4 id of the test data
		id := c4.Identify(strings.NewReader(key))
		testdata[key] = id

		// Test Validating store `Create` method
		w, err := st.Create(id)
		if err != nil {
			t.Fatal(err)
		}
		actualKey := key
		if i == randIndx {
			key = "bad data"
		}
		// Write data to the Validating store
		_, err = w.Write([]byte(key))
		if err != nil {
			t.Fatal(err)
		}

		// Close the Validating store
		err = w.Close()
		if i == randIndx {
			if err != ErrInvalidID {
				t.Errorf("expected error on reader close.")
			}
			delete(testdata, actualKey)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}

	}

	// Test Validating store `Open` method
	for k, v := range testdata {

		f, err := st.Open(v)
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
			t.Errorf("wrong data read from file, expted %q, got %q", k, string(data))
		}
		err = f.Close()
		if err != nil {
			t.Errorf("reader close with error %s", err)
		}
	}

	var badID c4.ID

	// sneek in some wrong data
	idmap := (map[c4.ID][]byte)(*ramst)
	if len(idmap) != len(testdata) {
		t.Error("test data and ram store length do not match")
	}
	for k := range idmap {
		idmap[k] = []byte("bad data")
		badID = k
		break
	}

	// Test Validating store `Open` method
	for _, v := range testdata {
		f, err := st.Open(v)
		if err != nil {
			t.Error(err)
		}

		data := make([]byte, 512)
		_, err = f.Read(data)
		if err != nil {
			t.Error(err)
		}

		_ = badID
		err = f.Close()
		if v == badID {
			if err != ErrInvalidID {
				t.Errorf("expected error on reader close.")
			}
			continue
		}
		if err != nil {
			t.Errorf("reader close with error %s", err)
		}
	}

}
