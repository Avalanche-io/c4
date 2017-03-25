package os_test

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blang/vfs"

	"testing"

	"github.com/cheekybits/is"
)

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

type PrettyRune rune
type PrettyRunes []rune

func (s PrettyRunes) IndexOf(r PrettyRune) int {
	for i, x := range s {
		if x == rune(r) {
			return i
		}
	}
	return -1
}

func (r PrettyRune) In(s PrettyRunes) bool {
	if s.IndexOf(r) == -1 {
		return false
	}
	return true
}

func NewPrettyRune() PrettyRune {
	rn := rune(rand.Uint32() % utf8.MaxRune)
	for !unicode.IsPrint(rn) || !unicode.IsGraphic(rn) {
		rn = rune(rand.Uint32() % utf8.MaxRune)
	}
	return PrettyRune(rn)
}

func NewPrettyRunes(length int, reject func(r PrettyRune) bool) PrettyRunes {
	runes := ""
	if reject == nil {
		reject = func(PrettyRune) bool {
			return false
		}
	}
	for i := 0; i < length; i++ {
		r := NewPrettyRune()
		for reject(r) {
			r = NewPrettyRune()
		}
		runes += string(r)
	}
	return PrettyRunes(runes)
}

// Utility for building random paths of files and folders in memory
func makeFsTree(is is.I) (treebuilder, *counts) {
	disallowed := PrettyRunes([]rune{'/', '\\', '?', '%', '*', ':', '|', '"', '\'', '>', '<', '\000'})
	_ = disallowed
	// words := get20kwords()
	r := rand.New(rand.NewSource(0xc4))

	// We create pseudo random directory structure with the following limits.
	max_path_depth := uint32(10)                  // Maximum number of nested folders
	max_name_chars := uint32(20)                  // Maximum characters in a filename
	max_path_chars := uint32(max_name_chars * 10) // Maximum characters in a file path
	max_data := uint32(512)                       // Maximum amount of random data to put in files.
	max_items_per_folder := uint32(40)            // Maximum number of children per folder.

	// make n, m files and folders
	// walk into some folders and repeat until max depth
	var tree treebuilder
	file_data := make([]byte, max_data)
	item_count := uint32(0)
	var t_files, t_folders, t_max_depth, t_max_path uint32
	count := &counts{&item_count, &t_files, &t_folders, &t_max_depth, &t_max_path}
	tree = func(fs vfs.Filesystem, max_depth, max_total_items, depth uint32, path string, item_count uint32) uint32 {
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
			namelen := ((r.Uint32() % max_name_chars) / 2)
			// First 50% chars are ascii range to make sorting tests clear
			prefixlen := namelen / 2
			// if prefixlen%4 != 0 {
			//   prefixlen += 4 - prefixlen%4
			// }
			if prefixlen == 0 {
				prefixlen = 1
			}

			alpha := PrettyRunes("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuv")
			name := string(NewPrettyRunes(int(prefixlen), func(r PrettyRune) bool {
				return !r.In(alpha)
			}))

			name += string(NewPrettyRunes(int(namelen), func(r PrettyRune) bool {
				// return r.In(disallowed)
				return !r.In(alpha)
			}))

			// for uint32(len(name)) < namelen {
			//   rn := rune(r.Uint32() % utf8.MaxRune)
			//   //Loop until anonymous function returns true, because where is 'indexOf'?
			//   //FIX: I know this looks like ass, but I'll have to fix it later.
			//   for func() bool {
			//     for _, x := range disallowed {
			//       if rn == x || (!unicode.IsPrint(rn) || !unicode.IsGraphic(rn)) {
			//         return false
			//       }
			//     }
			//     return true
			//   }() {
			//     rn = rune(r.Uint32() % utf8.MaxRune)
			//   }
			//   name += string(rn)
			//   // if len(name) > 0 {
			//   //   name += "_"
			//   // }
			//   // name += words[int(r.Uint32()%uint32(len(words)))]
			// }

			this_path := filepath.Join(path, name)
			if uint32(len(path)) >= max_path_chars {
				continue
			}
			path_len := uint32(len(strings.Split(this_path, "/")))
			if path_len > t_max_path {
				t_max_path = path_len
			}
			if r.Uint32()%4 == 0 { // 1 in 4 items is a folder
				t_folders++
				err := vfs.MkdirAll(fs, this_path, 0700)
				is.NoErr(err)
				item_count = tree(fs, max_depth, max_total_items, depth+1, this_path, item_count)
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
	return tree, count
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

// Create temp folder and return function to delete it.
func setup(t *testing.T, test_name string) (is.I, string, func()) {
	is := is.New(t)
	prefix := fmt.Sprintf("c4_%s_tests", test_name)
	dir, err := ioutil.TempDir("/tmp", prefix)
	is.NoErr(err)
	return is, dir, func() { os.RemoveAll(dir) }
}
