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

	"github.com/cheekybits/is"

	"github.com/avalanche-io/c4/store"
)

// Create temp folder and return function to delete it.
func setup(t *testing.T, test_name string) (is.I, string, func()) {
	is := is.New(t)
	prefix := fmt.Sprintf("c4_%s_tests", test_name)
	dir, err := ioutil.TempDir("/tmp", prefix)
	is.NoErr(err)
	return is, dir, func() { os.RemoveAll(dir) }
}

func TestDirectorySaveLoad(t *testing.T) {
	is, dir, done := setup(t, "store")
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
	is.NoErr(err)

	// test io.Reader interface
	n, err := io.Copy(f, d)
	is.NoErr(err)
	is.Equal(n, 49)
	// close file
	err = f.Close()
	is.NoErr(err)

	// Open
	var d2 store.Directory
	f2, err := os.Open(path)
	is.NoErr(err)
	n, err = io.Copy(&d2, f2)
	is.NoErr(err)
	is.Equal(n, 49)
	// close file
	err = f2.Close()
	is.NoErr(err)

	is.Equal(len(d2), 10)
	for i, name := range d2 {
		is.Equal(name, fmt.Sprintf(format, i))
	}
}

func TestDirectorySorting(t *testing.T) {
	is := is.New(t)
	_ = is
	size := 22
	format := "foo %d"
	list := shuffle(format, size)

	var d store.Directory
	for _, name := range list {
		d.Insert(name)
	}
	for i, name := range d {
		is.Equal(name, fmt.Sprintf(format, i))
	}
	var d2 store.Directory
	for _, name := range list {
		d2 = append(d2, name)
	}
	// directory implements the sort interface, and sorts in 'natural' order instead of
	// the typical lexicographical order (i.e. the sequence of numbers are preserved).
	sort.Sort(d2)
	for i, name := range d2 {
		is.Equal(name, fmt.Sprintf(format, i))
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
