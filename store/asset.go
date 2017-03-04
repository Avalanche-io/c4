package store

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"

	assetpath "path"

	c4 "github.com/Avalanche-io/c4/id"
)

// Asset is the equivalent of an os.File, with added features for identification.
type Asset struct {
	name string
	key  []byte
	mode int
	f    *os.File
	st   abstract_storage_interface
	en   *c4.Encoder
	id   *c4.ID
}

// NewAsset creates an Asset associated with the native file system instead of a C4 Store.
func NewAsset(path string) *Asset {
	_, filename := assetpath.Split(path)
	return &Asset{
		name: filename,
		key:  []byte(path),
		st:   nil_storage{},
		en:   c4.NewEncoder(),
	}
}

// TODO: if a.id == nil try to find the ID in the C4 db.  If the asset is open, then
// return the Encoder.ID() of the asset. If the ID is nil, the file is not open, and there
// is no id in the database, then open the file and identify it.
func (a *Asset) ID() *c4.ID {
	return a.id
}

func (a *Asset) Close() error {
	// If the file is read only then simply close it and return.
	if a.mode == os.O_RDONLY {
		if a.id == nil {
			return noIdError("closed " + string(a.key))
		}
		return a.f.Close()
	}
	// Otherwise commit and update the directory.
	err := a.commit()
	if err != nil {
		return err
	}
	return a.st.updateDirectory(a.key)
}

// Commit saves the C4 ID to the db and renames the file to it's ID
func (a *Asset) commit() error {
	// Close the file
	err := a.f.Close()
	if err != nil {
		return err
	}

	id := a.en.ID()
	if id == nil {
		return noIdError(string(a.key))
	}
	// Compute ID
	a.id = id
	// Move identified file to it's storage location (if not already present)
	err = a.st.move(a.f.Name(), a.id)
	if err != nil {
		return err
	}
	return a.st.set(a.key, a.id)
}

// Name returns the name of the asset.
func (a *Asset) Name() string {
	return a.name
}

// Read reads up to len(b) bytes from the Asset. It returns the number of bytes read and
// any error encountered. At end of file, Read returns 0, io.EOF.
func (a *Asset) Read(b []byte) (n int, err error) {
	return a.f.Read(b)
}

// ReadAt reads len(b) bytes from the Asset starting at byte offset off. It returns the
// number of bytes read and the error, if any. ReadAt always returns a non-nil error when
// n < len(b). At end of file, that error is io.EOF.
func (a *Asset) ReadAt(b []byte, off int64) (n int, err error) {
	return a.f.ReadAt(b, off)
}

// Seek sets the offset for the next Read or Write on file to offset, interpreted
// according to whence: 0 means relative to the origin of the file, 1 means relative to
// the current offset, and 2 means relative to the end. It returns the new offset and an
// error, if any.
func (a *Asset) Seek(offset int64, whence int) (ret int64, err error) {
	return a.f.Seek(offset, whence)
}

// Readdir reads the contents of the directory associated with the asset and returns
// a slice of up to n FileInfo values, in directory order. Subsequent calls on the same
// file will yield further FileInfos if any.

// If n > 0, Readdir returns at most n FileInfos. In this case, if Readdir returns an
// empty slice, it will return a non-nil error explaining why. At the end of a directory,
// the error is io.EOF.

// If n <= 0, Readdir returns all the FileInfo from the directory in a single
// slice. In this case, if Readdir succeeds to read the complete directory, it returns
// the slice and a nil error. If it encounters an error before the end of the directory,
// Readdir returns the FileInfos read until that point and a non-nil error.
func (a *Asset) Readdir(n int) (files []os.FileInfo, err error) {
	// read file
	// get list of strings (dir names) a.Readdirnames
	// create os.FileInfo array by loading metadata for each key
	// a.f.Read()
	err = errors.New("unimplemented")
	return
}

// Readdirnames reads and returns a slice of names from the directory f.
//
// If n > 0, Readdirnames returns at most n names. In this case, if Readdirnames
// returns an empty slice, it will return a non-nil error explaining why. At the
// end of a directory, the error is io.EOF.
//
// If n <= 0, Readdirnames returns all the names from the directory in a single
// slice. In this case, if Readdirnames succeeds (reads all the way to the end
// of the directory), it returns the slice and a nil error. If it encounters an
// error before the end of the directory, Readdirnames returns the names read
// until that point and a non-nil error.
func (a *Asset) Readdirnames(n int) (names []string, err error) {
	forever := true
	if n > 0 {
		names = make([]string, 0, n)
		forever = false
	}
	scanner := bufio.NewScanner(a.f)
	scanner.Split(directoryscanner)
	i := 0
	for scanner.Scan() && (i < n || forever) {
		err = scanner.Err()
		names = append(names, scanner.Text())
		if err != nil {
			return
		}
		i++
	}
	return
}

func (a *Asset) Write(b []byte) (n int, err error) {
	w := io.MultiWriter(a.en, a.f)
	n64, er := io.Copy(w, bytes.NewReader(b))
	return int(n64), er
}

func (a *Asset) WriteAt(b []byte, off int64) (n int, err error) {
	// TODO: WriteAt cannot ID with MultiWRiter
	return a.f.WriteAt(b, off)
}

func (a *Asset) WriteString(s string) (n int, err error) {
	return a.Write([]byte(s))
}

func (a *Asset) Stat() (info os.FileInfo, err error) {
	err = errors.New("unimplemented")
	return
}

// asset_copy is a convenience function that returns a shallow copy of an asset.
func asset_copy(a *Asset) *Asset {
	return &Asset{
		name: a.name,
		key:  a.key,
		mode: a.mode,
		f:    a.f,
		st:   a.st,
		en:   a.en,
		id:   a.id,
	}
}
