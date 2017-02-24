package db_test

import (
	// "errors"

	// "github.com/Avalanche-io/c4/events"

	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	c4db "github.com/Avalanche-io/c4/db"
	c4id "github.com/Avalanche-io/c4/id"
	"github.com/cheekybits/is"
)

// Create temp folder and return function to delete it.
func SetupTestFolder(t *testing.T, test_name string) (is.I, string, func()) {
	is := is.New(t)
	prefix := fmt.Sprintf("c4_%s_tests", test_name)
	dir, err := ioutil.TempDir("/tmp", prefix)
	is.NoErr(err)
	return is, dir, func() {
		os.RemoveAll(dir)
		// fmt.Printf("os.RemoveAll(%s)\n", dir)
	}
}

func TestStoreSaveLoad(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	// defer done()
	_ = done

	st, err := c4db.OpenStorage(dir + "/asset_storage")
	is.NoErr(err)
	is.NotNil(st)

	asset, err := st.Create("/foo.txt")
	is.NoErr(err)
	is.NotNil(asset)

	n, err := asset.Write([]byte("foo"))
	is.NoErr(err)
	is.Equal(n, 3)
	err = asset.Close()
	is.NoErr(err)
	id := asset.ID()
	is.NotNil(id)

	fooId, err := c4id.Identify(strings.NewReader("foo"))
	is.NoErr(err)
	is.Equal(fooId, id)
	err = st.Close()
	is.NoErr(err)

	st2, err := c4db.OpenStorage(dir + "/asset_storage")
	is.NoErr(err)
	is.NotNil(st)

	asset2, err := st2.Open("/foo.txt")
	is.NoErr(err)
	is.NotNil(asset2)
	is.NotNil(asset2.ID())
	st2.Close()

	path_to_assetfile := filepath.Join(dir, "/asset_storage/c4/5x/Ze/Xw/MS/pq/Xj/pD/", id.String())

	info, err := os.Stat(path_to_assetfile)
	is.NoErr(err)
	is.NotNil(info)
	is.Equal(info.Size(), 3)
}

func TestStoreDirs(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()
	_ = done
	st, err := c4db.OpenStorage(dir + "/asset_storage")
	is.NoErr(err)
	is.NotNil(st)

	err = st.Mkdir("/dir1")
	is.NoErr(err)
	err = st.MkdirAll("/dir2/foo/bar/baz")
	is.NoErr(err)
	asset, err := st.Create("/dir2/foo/bar/foo.txt")
	is.NoErr(err)
	is.NotNil(asset)

	n, err := asset.Write([]byte("foo"))
	is.NoErr(err)
	is.Equal(n, 3)
	err = asset.Close()
	is.NoErr(err)

	folder_asset, err := st.Open("/dir2/foo/bar/")
	is.NoErr(err)
	names, err := folder_asset.Readdirnames(-1)
	is.NoErr(err)
	expected := []string{
		"baz/",
		"foo.txt",
	}
	is.Equal(len(names), len(expected))
	for i, name := range names {
		is.True(i < len(expected))
		is.Equal(expected[i], name)
	}

	err = folder_asset.Close()
	is.NoErr(err)

}
