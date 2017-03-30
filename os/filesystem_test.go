package os_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/vfs"
	"github.com/blang/vfs/memfs"

	"testing"

	c4id "github.com/avalanche-io/c4/id"
	c4os "github.com/avalanche-io/c4/os"
	"github.com/cheekybits/is"
)

func TestFSWalk(t *testing.T) {
	is, dir, done := setup(t, "os")
	defer done()
	paths := [][]string{
		strings.Split("one/two/three", "/"),
	}
	filename := "testfile.txt"

	for _, p := range paths {
		path := filepath.Join(p...)
		path = filepath.Join(dir, path)
		err := os.MkdirAll(path, 0700)
		is.NoErr(err)
		file_path := filepath.Join(path, filename)
		f, err := os.Create(file_path)
		is.NoErr(err)
		f.Write([]byte("foo"))
		id := c4id.Identify(bytes.NewReader([]byte("foo")))
		is.Equal(id.String(), "c45xZeXwMSpqXjpDumcHMA6mhoAmGHkUo7r9WmN2UgSEQzj9KjgseaQdkEJ11fGb5S1WEENcV3q8RFWwEeVpC7Fjk2")
		f.Close()
	}

	type testT struct {
		Key  string
		Id   string
		Info os.FileInfo
	}

	var tests [][]testT

	for _, p := range paths {
		var test_set []testT
		p = append(p, filename)
		p = append([]string{dir}, p...)
		for j, _ := range p {
			path := filepath.Join(p[:len(p)-j]...)
			info, err := os.Stat(path)
			is.NoErr(err)
			test := testT{
				Key:  path,
				Id:   "c45xZeXwMSpqXjpDumcHMA6mhoAmGHkUo7r9WmN2UgSEQzj9KjgseaQdkEJ11fGb5S1WEENcV3q8RFWwEeVpC7Fjk2",
				Info: info,
			}
			test_set = append(test_set, test)
		}
		tests = append(tests, test_set)
	}
	_ = tests

	c4fs := c4os.NewFileSystem(vfs.OS(), []byte(dir))
	err := c4fs.Walk(nil, func(key []byte, attrs c4os.Attributes) error {
		id := attrs.ID()
		is.NotNil(id)
		// since there is only one file in this file system structure there is only one id
		is.Equal(id.String(), "c45xZeXwMSpqXjpDumcHMA6mhoAmGHkUo7r9WmN2UgSEQzj9KjgseaQdkEJ11fGb5S1WEENcV3q8RFWwEeVpC7Fjk2")
		var info os.FileInfo
		err := attrs.Get([]byte("info"), &info)
		is.NoErr(err)
		is.NotNil(info)
		info2, err := os.Stat(string(key))
		is.NoErr(err)
		is.Equal(info2.Name(), info.Name())
		is.Equal(info2.IsDir(), info.IsDir())
		is.Equal(info2.Mode(), info.Mode())
		is.Equal(info2.ModTime().Format(time.RFC3339), info.ModTime().Format(time.RFC3339))
		return nil
	})
	is.NoErr(err)
	// data, err := json.Marshal(c4fs)
	// is.NoErr(err)
	// fmt.Printf("c4fs: %s\n", string(data))
}

// Test the canonical identification of a folder.
// A folder consists of two content (i.e. non-metadata) elements
// 1. A list of files and folders
// 2. Content of those files and folders
// Metadata for the folder, and for it's children is separately identified
// because one can construct the contents completely without it, and so it
// relates to archival operations, but not to core identification.
// testing
func TestDirID(t *testing.T) {
	is := is.New(t)
	tree, _ := makeFsTree(is)
	fs := memfs.Create()
	item_count := tree(fs, 8, 20, 0, "/", 0)
	c4fs := c4os.NewFileSystem(fs, []byte("/"))
	cnt := 0
	err := c4fs.Walk(nil, func(key []byte, attrs c4os.Attributes) error {
		cnt++
		return nil
	})
	is.NoErr(err)
	is.Equal(cnt, item_count)
	data, err := json.Marshal(c4fs)
	is.NoErr(err)
	// i := 0
	// for k := range c4fs.Keys() {
	// fmt.Printf("%d: %s\n", i, k)
	// i++
	// }
	_ = data
	// fmt.Printf("%s\n", data)
	// fmt.Printf("tree: item_count: %d, Total files: %d, Total folders: %d, Total MAX Depth: %d, MAX Path: %d\n", item_count, *count.t_files, *count.t_folders, *count.t_max_depth, *count.t_max_path)
}
