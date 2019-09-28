package store_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Avalanche-io/c4/store"
)

// updated, delete me

// Create temp folder and return function to delete it.
func setup(t *testing.T, test_name string) (string, func()) {

	prefix := fmt.Sprintf("c4_%s_tests", test_name)
	dir, err := ioutil.TempDir("", prefix)
	if err != nil {

	}
	return dir, func() { os.RemoveAll(dir) }
}

func TestDirectorySaveLoad(t *testing.T) {
	dir, done := setup(t, "store")
	defer done()
	path := filepath.Join(dir, "test_director.dir")
	format := "%04d"

	// Create
	var d store.Directory
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf(format, i)
		d = append(d, name)
	}

	// Open file
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("unable to create %q: %s", path, err)
	}

	// test io.Reader interface
	n, err := io.Copy(f, d)
	if err != nil {
		t.Fatalf("unable to copy Directory: %s", err)
	}
	if n != 49 {
		t.Errorf("incorrect result expecting %d got %d\n", 49, n)
	}

	// close file
	err = f.Close()
	if err != nil {
		t.Errorf("Crror closing file: %s", err)
	}

	// Open
	var d2 store.Directory
	f2, err := os.Open(path)
	if err != nil {
		t.Fatalf("unable to create %q: %s", path, err)
	}

	n, err = io.Copy(&d2, f2)
	if err != nil {
		t.Fatalf("unable to copy Directory: %s", err)
	}
	if n != 49 {
		t.Errorf("incorrect result expecting %d got %d\n", 49, n)
	}

	// close file
	err = f2.Close()
	if err != nil {
		t.Errorf("error closing file: %s", err)
	}

	if len(d2) != 10 {
		t.Errorf("incorrect result expecting %d got %d\n", 10, len(d2))
	}

	for i, name := range d2 {
		if name != fmt.Sprintf(format, i) {
			t.Errorf("incorrect result at %d expecting got %d\n", fmt.Sprintf(format, i), name)
		}
	}
}

func TestDirectorySorting(t *testing.T) {
	size := 22
	format := "foo %d"
	list := shuffle(format, size)

	var d store.Directory
	for _, name := range list {
		d.Insert(name)
	}
	for i, name := range d {
		if name != fmt.Sprintf(format, i) {
			t.Errorf("incorrect result at %d expecting got %d\n", fmt.Sprintf(format, i), name)
		}
	}
	var d2 store.Directory
	for _, name := range list {
		d2 = append(d2, name)
	}
	// directory implements the sort interface, and sorts in 'natural' order instead of
	// the typical lexicographical order (i.e. the sequence of numbers are preserved).
	sort.Sort(d2)
	for i, name := range d2 {
		if name != fmt.Sprintf(format, i) {
			t.Errorf("incorrect result at %d expecting got %d\n", fmt.Sprintf(format, i), name)
		}
	}
}

func shuffle(format string, size int) []string {
	list := make([]string, size)
	for j, i := 0, 0; i < size; i++ {
		if i > 0 {
			j = int(rand.Int31n(int32(i)))
		}
		if j != i {
			list[i] = list[j]
		}
		list[j] = fmt.Sprintf(format, i)
	}
	return list
}
