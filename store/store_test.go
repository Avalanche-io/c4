package store_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	c4id "github.com/Avalanche-io/c4/id"
	c4store "github.com/Avalanche-io/c4/store"
	"github.com/cheekybits/is"
)

// Create temp folder and return function to delete it.
func SetupTestFolder(t *testing.T, test_name string) (is.I, string, func()) {
	is := is.New(t)
	prefix := fmt.Sprintf("c4_%s_tests", test_name)
	dir, err := ioutil.TempDir("/tmp", prefix)
	is.NoErr(err)
	return is, dir, func() { os.RemoveAll(dir) }
}

func TestStoreSaveLoad(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()
	path := filepath.Join(dir, "storage")
	st, err := c4store.Open(path)
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

	fooId := c4id.Identify(strings.NewReader("foo"))
	is.Equal(fooId, id)
	err = st.Close()
	is.NoErr(err)

	st2, err := c4store.Open(path)
	is.NoErr(err)
	is.NotNil(st)

	asset2, err := st2.Open("/foo.txt")
	is.NoErr(err)
	is.NotNil(asset2)
	is.NotNil(asset2.ID())
	st2.Close()

	path_to_assetfile := filepath.Join(dir, "storage/c4/5x/Ze/Xw/MS/pq/Xj/pD/", id.String())

	info, err := os.Stat(path_to_assetfile)
	is.NoErr(err)
	is.NotNil(info)
	is.Equal(info.Size(), 3)
}

func TestStoreDirs(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()
	_ = done
	st, err := c4store.Open(dir + "/store")
	is.NoErr(err)
	is.NotNil(st)

	err = st.Mkdir("/dir1/")
	is.NoErr(err)
	err = st.MkdirAll("/dir2/foo/bar/baz/")
	is.NoErr(err)
	asset, err := st.Create("/dir2/foo/bar/foo.txt")
	is.NoErr(err)
	is.NotNil(asset)
	asset2, err := st.Create("/dir2/foo/bar/cat.txt")
	is.NoErr(err)
	is.NotNil(asset2)

	n, err := asset.Write([]byte("foo"))
	is.NoErr(err)
	is.Equal(n, 3)
	err = asset.Close()
	is.NoErr(err)

	n, err = asset2.WriteString("bar")
	is.NoErr(err)
	is.Equal(n, 3)
	n, err = asset2.WriteAt([]byte("bar"), 2) // "babar"
	err = asset2.Close()
	is.NoErr(err)

	is.Equal(asset.Name(), "foo.txt")
	is.Equal(asset2.Name(), "cat.txt")

	asset3, err := st.Open("/dir2/foo/bar/cat.txt")
	is.NoErr(err)
	data := make([]byte, 512)
	n, err = asset3.Read(data)
	is.NoErr(err)
	is.Equal(n, len("babar"))
	data = data[:n]
	is.Equal(string(data), "babar")
	ret, err := asset3.Seek(0, os.SEEK_SET)
	is.NoErr(err)
	is.Equal(ret, 0)
	data2 := make([]byte, 512)
	n, err = asset3.ReadAt(data2, 1)
	is.Equal(err, io.EOF)
	is.Equal(n, len("abar"))
	data2 = data2[:n]
	is.Equal(string(data2), "abar")

	folder_asset, err := st.Open("/dir2/foo/bar/")
	is.NoErr(err)
	names, err := folder_asset.Readdirnames(-1)
	is.NoErr(err)
	expected := []string{
		"baz/",
		"cat.txt",
		"foo.txt",
	}
	is.Equal(len(names), len(expected))
	for i, name := range names {
		is.True(i < len(expected))
		is.Equal(expected[i], name)
	}
	folder_asset.Seek(0, os.SEEK_SET)
	names2, err := folder_asset.Readdirnames(2)
	is.NoErr(err)
	expected2 := []string{
		"baz/",
		"cat.txt",
	}
	is.Equal(len(names2), len(expected2))
	for i, name := range names2 {
		is.True(i < len(expected2))
		is.Equal(expected2[i], name)
	}

	// Not yet implemented
	filesinfo, err := folder_asset.Readdir(-1)
	is.Err(err)
	is.Nil(filesinfo)

	err = folder_asset.Close()
	is.NoErr(err)

}

func TestErrors(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()

	// Setup

	unwriteableFilepath := filepath.Join(dir, "unwriteableFile")
	unwriteableFolderpath := filepath.Join(dir, "unwriteableFolder")
	unwriteableDbfolder := filepath.Join(dir, "unwriteableDbfolder")
	unwriteableDbpath := filepath.Join(unwriteableDbfolder, "c4id.db")
	os.Mkdir(unwriteableFolderpath, 0000)
	os.Mkdir(unwriteableDbfolder, 0777)
	f, err := os.Create(unwriteableFilepath)
	is.NoErr(err)
	data := "foo"
	n, err := f.Write([]byte(data))
	is.NoErr(err)
	is.Equal(n, len(data))
	f.Close()
	err = os.Chmod(unwriteableFilepath, 0000)
	is.NoErr(err)
	f, err = os.Create(unwriteableDbpath)
	is.NoErr(err)
	f.Close()
	err = os.Chmod(unwriteableDbpath, 0000)
	is.NoErr(err)

	st, err := c4store.Open(unwriteableFilepath)
	is.Err(err)
	is.Nil(st)

	st, err = c4store.Open(unwriteableFolderpath)
	is.Err(err)
	is.Nil(st)

	st, err = c4store.Open(unwriteableDbfolder)
	is.Err(err)
	is.Nil(st)
}

func TestWriter(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()

	st, err := c4store.Open(dir + "/store")
	is.NoErr(err)
	is.NotNil(st)
	w, err := st.Writer("/foo")
	is.NoErr(err)
	_, err = io.Copy(w, bytes.NewReader([]byte("bar")))
	w.Close()
	bar_id := c4id.Identify(bytes.NewReader([]byte("bar")))
	is.Equal(w.ID().String(), bar_id.String())
	asset, err := st.Open("/foo")
	is.NoErr(err)
	defer asset.Close()
	is.Equal(asset.ID().String(), bar_id.String())
}

func TestReaderWriter(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()

	st, err := c4store.Open(dir + "/store")
	is.NoErr(err)
	is.NotNil(st)
	w, err := st.Writer("/foo")
	is.NoErr(err)
	_, err = io.Copy(w, bytes.NewReader([]byte("bar")))
	w.Close()
	bar_id := c4id.Identify(bytes.NewReader([]byte("bar")))
	is.Equal(w.ID().String(), bar_id.String())
	w.Close()
	r, err := st.Reader("/foo")
	is.NoErr(err)
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	is.NoErr(err)
	is.True(string(data) == "bar")
	is.Equal(r.ID().String(), bar_id.String())
	r.Close()
}
