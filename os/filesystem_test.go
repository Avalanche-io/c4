package os_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/vfs"
	"github.com/blang/vfs/memfs"

	"testing"

	c4id "github.com/Avalanche-io/c4/id"
	c4os "github.com/Avalanche-io/c4/os"
	// "github.com/Avalanche-io/c4dev"
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

// TestFSWalk tests walking folder trees.
// Philosophically we want C4 to return as much data as it can as soon as it can.
// C4 must do a depth first traversal to compute IDs for folders.
// As it traverses down it should report name and metadata, and id files it encounters.
// once all the files of a folder are identified the folder should be identified.
func TestFSWalk(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "filesystem")
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
		id, err := c4id.Identify(bytes.NewReader([]byte("foo")))
		is.NoErr(err)
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
	deeper, _ := makeRamTree(is)
	fs := memfs.Create()
	item_count := deeper(fs, 8, 20, 0, "/", 0)
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
	// fmt.Printf("deeper: item_count: %d, Total files: %d, Total folders: %d, Total MAX Depth: %d, MAX Path: %d\n", item_count, *count.t_files, *count.t_folders, *count.t_max_depth, *count.t_max_path)
}

type treebuilder func(fs vfs.Filesystem, max_depth, max_total_items, depth uint32, path string, item_count uint32) uint32

type counts struct {
	item_count, t_files, t_folders, t_max_depth, t_max_path *uint32
}

func get20kwords() []string {
	var words []string
	fin, err := os.Open("test_data/20k.txt")
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(fin)
	for scanner.Scan() {
		words = append(words, scanner.Text()) // Println will add back the final '\n'
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	return words
}

// Utility for building random paths of files and folders in memory
func makeRamTree(is is.I) (treebuilder, *counts) {
	// disallowed := []byte{'/', '\\', '?', '%', '*', ':', '|', '"', '\'', '>', '<', '\000'}
	// _ = disallowed
	words := get20kwords()
	r := rand.New(rand.NewSource(0xc4))

	// We create pseudo random directory structure with the following limits.
	max_path_depth := uint32(10)                  // Maximum number of nested folders
	max_name_chars := uint32(40)                  // Maximum characters in a filename
	max_path_chars := uint32(max_name_chars * 10) // Maximum characters in a file path
	max_data := uint32(10)                        // Maximum amount of random data to put in files.
	max_items_per_folder := uint32(40)            // Maximum number of children per folder.

	// make n, m files and folders
	// walk into some folders and repeat until max depth
	var deeper treebuilder
	file_data := make([]byte, max_data)
	item_count := uint32(0)
	var t_files, t_folders, t_max_depth, t_max_path uint32
	count := &counts{&item_count, &t_files, &t_folders, &t_max_depth, &t_max_path}
	deeper = func(fs vfs.Filesystem, max_depth, max_total_items, depth uint32, path string, item_count uint32) uint32 {
		if item_count >= max_total_items {
			return item_count
		}
		if depth >= max_depth || (r.Uint32()%(max_path_depth/2)) == 0 {
			if depth >= max_depth {
				t_max_depth++
			}
			return item_count
		}
		folder_items := r.Uint32() % max_items_per_folder
		for i := uint32(0); i < folder_items; i++ {
			item_count++
			if item_count >= max_total_items {
				return item_count
			}
			var name string
			for uint32(len(name)) < max_name_chars {
				if len(name) > 0 {
					name += "_"
				}
				name += words[int(r.Uint32()%uint32(len(words)))]
			}

			this_path := filepath.Join(path, name)
			if uint32(len(path)) >= max_path_chars {
				continue
			}
			path_len := uint32(len(strings.Split(this_path, "/")))
			if path_len > t_max_path {
				t_max_path = path_len
			}
			if r.Uint32()%2 == 0 { // 1 in 2 items is a folder
				t_folders++
				err := vfs.MkdirAll(fs, this_path, 0700)
				is.NoErr(err)
				item_count = deeper(fs, max_depth, max_total_items, depth+1, this_path, item_count)
				if item_count >= max_total_items {
					return item_count
				}
				continue
			}
			t_files++
			n, err := r.Read(file_data)
			is.NoErr(err)
			is.Equal(n, uint32(len(file_data)))
			err = vfs.WriteFile(fs, this_path, file_data[:r.Uint32()%max_data], 0600)
			is.NoErr(err)
			if item_count >= max_total_items {
				return item_count
			}
		}
		return item_count
	}
	return deeper, count
}

func makePrintTree(fs vfs.Filesystem, is is.I) func(int, string, os.FileInfo) {
	var printTree func(int, string, os.FileInfo)
	printTree = func(depth int, path string, info os.FileInfo) {
		padding := strings.Repeat("  ", depth)
		if info.IsDir() {
			fmt.Printf("%s%s/ # %s\n", padding, info.Name(), path)
			dir, err := fs.ReadDir(path)
			is.NoErr(err)
			for _, i := range dir {
				printTree(depth+1, filepath.Join(path, i.Name()), i)
			}
			return
		}
		fmt.Printf("%s%s %d\n", padding, info.Name(), info.Size())
	}
	return printTree
}
