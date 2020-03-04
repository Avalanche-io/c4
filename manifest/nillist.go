package manifest

import (
	"bytes"
	"sort"
	"strings"

	"github.com/xtgo/set"
)

// Replaces path seperators with a zero byte, it works on the strings in place
// with out creating a new slice.
func newNilList(list []string) nilList {
	blist := make([][]byte, len(list))
	for i, path := range list {
		blist[i] = fromSlash(path)
	}
	ml := nilList(blist)
	sort.Sort(ml)
	n := set.Uniq(ml)
	ml = ml[:n]
	return ml
}

type nilList [][]byte

func (l nilList) Len() int           { return len(l) }
func (l nilList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l nilList) Less(i, j int) bool { return bytes.Compare(l[i], l[j]) == -1 }

func (l nilList) StringSlice() []string {
	list := make([]string, l.Len())
	for i, path := range l {
		list[i] = toSlash(path)
	}
	return list
}

func (l nilList) Reverse() {
	// reverse the nil list to produce a post-order traversal for c4 id
	for i, j := 0, l.Len()-1; i < l.Len()/2; i, j = i+1, j-1 {
		l.Swap(i, j)
	}
}

func (l nilList) Get(i int) string {
	return toSlash(l[i])
}

// Converts the string to bytes and replaces "/" with 0
func fromSlash(path string) []byte {
	if len(path) == 0 {
		return []byte{}
	}
	data := make([]byte, len(path))
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			data[i] = 0
			continue
		}
		data[i] = path[i]
	}
	return data
}

// Replaces all 0 with "/"
func toSlash(path []byte) string {
	if path == nil || len(path) == 0 {
		return ""
	}
	var strBuilder strings.Builder
	strBuilder.Grow(len(path))

	// data := make([]byte, len(path))
	for i := range path {
		if path[i] != 0 {
			strBuilder.WriteByte(path[i])
			continue
		}
		strBuilder.WriteByte('/')
	}
	return strBuilder.String()
}

func Diff(a nilList, b nilList) nilList {
	ab := append(a, b...)
	n := set.Diff(ab, len(a))
	return ab[:n]
}

// Find returns the index of the first occurrence of path in the pathlist, or
// where the path would would be inserted if it does not exist
func (l nilList) Find(key []byte) int {

	return sort.Search(l.Len(), func(i int) bool {
		return bytes.Compare(l[i], key) >= 0
	})

}

// End is like Find but returns the index just after the last matching `prefix`.
// The first path in `pathlist` must be the first matching path as returned
// by Find. End returns a value form 0 to len(pathlist).
//
// Typical usage would be to call Find first to get the starting index, then
// call End with a slice of the list starting at that index.
//
// For example:
//
//   prefix := "/some/path"
//   start := pathlist.Find(paths, prefix)
//   if start == -1 {
// 	   return paths[:0]
//   }
//
//   end := pathlist.End(paths[start:], prefix)
//   return paths[start:start+end]
//
func (l nilList) End(key []byte) int {

	return sort.Search(l.Len(), func(i int) bool {
		return !bytes.HasPrefix(l[i], key)
	})

}

// Children returns the list of child names for a given path. The results
// are unique and in sorted order. This operation runs in O(log N * M) time where
// `N` is the total number of decedents of `path`, and M is the number of sub
// folders.
func (l nilList) Children(key []byte) nilList {
	var out nilList

	list := l.Sublist(key)
	// if path == "/" {
	// 	path = ""
	// }

	length := len(key)
	for list.Len() > 0 {

		if len(list[0]) == length || list[0][length] != 0 {
			break
		}
		prefix := list[0]
		list = list[1:]

		// The child name ends either at the end of the key, or on the next nil seperator
		i := bytes.IndexByte(prefix[length+1:], 0)

		if i >= 0 {
			i = i + length
			prefix = prefix[:i+1]
		}

		out = append(out, prefix)
		end := list.End(append(prefix, 0))
		list = list[end:]
	}
	return out
}

func trimFront(key []byte) ([]byte, []byte) {
	i := bytes.Index(key, []byte{0})
	for i > -1 {
		if i == 0 {
			key = key[1:]
			i = bytes.Index(key, []byte{0})
			continue
		}
		return key[:i], key[i:]
	}
	return key, nil
}

// Sublist returns a new Pathlist containing only decedents of `path`. If
// Pathlist does not contain an exact match of `path` nil is returned.
func (l nilList) Sublist(key []byte) nilList {

	s := l.Find(key)
	if s == l.Len() {
		return nilList{}
	}

	return l[s:l.End(key)]
}
