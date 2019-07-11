package store

import (
	"bufio"
	"bytes"
	"io"
	"sort"

	"github.com/Avalanche-io/c4/lang"
)

// updated, delete me

// Directory is a slice of strings that represents the list of names that are children
// of the directory.  It only provides the names, to get more information the names must
// be composed into paths.
type Directory []string

// Directory implements the sort interface.  Sort order is 'natural' order, not
// lexicographical.
func (d Directory) Len() int           { return len(d) }
func (d Directory) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d Directory) Less(i, j int) bool { return lang.Ordered(d).Less(i, j) }

// Directory implements io.Reader interface for writing the directory to a byte slice.
// The format is a list of strings separated by byte(0)
func (d Directory) Read(b []byte) (n int, err error) {
	cursor := 0
	for i, name := range d {
		nil_byte := 0
		if i < len(d)-1 {
			nil_byte = 1
		}
		if (cursor + len(name) + nil_byte) > len(b) {
			return cursor, nil
		}
		copy(b[cursor:], []byte(name))
		cursor += len(name)
		if nil_byte > 0 {
			b[cursor] = byte(0)
			cursor++
		}

	}
	return cursor, io.EOF
}

// Directory implements io.Writer interface for reading the directory from a byte slice
// The format is a list of strings separated by byte(0)
func (d *Directory) Write(p []byte) (n int, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(p))
	scanner.Split(directoryscanner)
	var entry string
	for scanner.Scan() {
		err = scanner.Err()
		entry = scanner.Text()
		n += len(entry) + 1
		d.Insert(entry)
		if err != nil {
			return
		}
	}
	if n > 0 {
		n -= 1
	}
	return
}

// insert adds names to the dictionary ensuring ascending 'natural' order and
// uniqueness.
func (d *Directory) Insert(name string) {
	if len(name) == 0 {
		return
	}
	i := d.Index(name)
	if i < len(*d) && (*d)[i] == name {
		return
	}
	*d = append(*d, "")
	copy((*d)[i+1:], (*d)[i:])
	(*d)[i] = name
}

// Index returns the location of x in the list of names, or the index at which
// x would be inserted into the slice if it is not in the set.
func (d Directory) Index(x string) int {
	return sort.Search(len(d), func(i int) bool { return lang.NaturalCompare(d[i], x) >= 0 })
}

// directoryscanner implements bufio.SplitFunc.  It tokenizes byte(0) delimited
// strings.  An error is returned if a string of length 0 is parsed (i.e. two
// byte(0)s in a row)
func directoryscanner(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == byte(0) {
			if i == 0 {
				return 0, nil, ErrDirUnderflow
			}
			return i + 1, data[:i], nil
		}
	}
	// no token returned yet
	switch {
	case len(data) == 0 && atEOF:
		return 0, nil, ErrDirUnderflow
	case len(data) != 0 && atEOF:
		return len(data), data, nil
	case len(data) == 0 && !atEOF:
		return 0, nil, io.EOF
	case len(data) != 0 && !atEOF:
		return 0, nil, nil
	}

	return 0, nil, nil
}
