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
type Asset interface {
	ID() *c4.ID
	Name() string
	Read(b []byte) (n int, err error)
	ReadAt(b []byte, off int64) (n int, err error)
	Readdir(n int) (files []os.FileInfo, err error)
	Readdirnames(n int) (names []string, err error)
	Seek(offset int64, whence int) (ret int64, err error)
	Stat() (info os.FileInfo, err error)
	Storage() abstract_storage_interface
	Write(b []byte) (n int, err error)
	WriteAt(b []byte, off int64) (n int, err error)
	WriteString(s string) (n int, err error)
	Close() error
	commit() error
	Remove()
}

type file_asset struct {
	name string
	key  []byte
	mode int
	f    *os.File
	st   abstract_storage_interface
	en   *c4.Encoder
	id   *c4.ID
}

type folder_asset struct {
	name string
	key  []byte
	mode int
	f    *os.File
	st   abstract_storage_interface
	en   *c4.Encoder
	id   *c4.ID
}

// func (a *folder_asset) Close() error
func (a *folder_asset) commit() error {
	return (*file_asset)(a).commit()
}
func (a *folder_asset) ID() *c4.ID {
	return (*file_asset)(a).ID()
}
func (a *folder_asset) Name() string {
	return (*file_asset)(a).Name()
}
func (a *folder_asset) Read(b []byte) (n int, err error) {
	return (*file_asset)(a).Read(b)
}
func (a *folder_asset) ReadAt(b []byte, off int64) (n int, err error) {
	return (*file_asset)(a).ReadAt(b, off)
}
func (a *folder_asset) Readdir(n int) (files []os.FileInfo, err error) {
	return (*file_asset)(a).Readdir(n)
}
func (a *folder_asset) Readdirnames(n int) (names []string, err error) {
	return (*file_asset)(a).Readdirnames(n)
}
func (a *folder_asset) Seek(offset int64, whence int) (ret int64, err error) {
	return (*file_asset)(a).Seek(offset, whence)
}
func (a *folder_asset) Stat() (info os.FileInfo, err error) {
	return (*file_asset)(a).Stat()
}
func (a *folder_asset) Storage() abstract_storage_interface {
	return (*file_asset)(a).Storage()
}
func (a *folder_asset) Write(b []byte) (n int, err error) {
	return (*file_asset)(a).Write(b)
}
func (a *folder_asset) WriteAt(b []byte, off int64) (n int, err error) {
	return (*file_asset)(a).WriteAt(b, off)
}
func (a *folder_asset) WriteString(s string) (n int, err error) {
	return (*file_asset)(a).WriteString(s)
}
func (a *folder_asset) Remove() {
	(*file_asset)(a).Remove()
}

func NewDirAsset(path string, st abstract_storage_interface, mode int, f *os.File) (Asset, error) {
	_, filename := assetpath.Split(path)
	return &folder_asset{
		name: filename,
		key:  []byte(path),
		mode: mode,
		f:    f,
		st:   st,
		en:   c4.NewEncoder(),
	}, nil
}

func NewFileAsset(path string, st abstract_storage_interface, mode int, f *os.File) (Asset, error) {
	_, filename := assetpath.Split(path)
	return &file_asset{
		name: filename,
		key:  []byte(path),
		mode: mode,
		f:    f,
		st:   st,
		en:   c4.NewEncoder(),
	}, nil
}

func CopyAsset(a Asset, st abstract_storage_interface) Asset {
	switch val := a.(type) {
	case *file_asset:
		return &file_asset{
			name: val.name,
			key:  val.key,
			mode: val.mode,
			f:    val.f,
			st:   st,
			en:   val.en,
			id:   val.id,
		}
	}
	return nil
}

// NewAsset creates an Asset associated with the native file system instead of a C4 Store.
// func NewAsset(path string) Asset {
// 	_, filename := assetpath.Split(path)
// 	return &file_asset{
// 		name: filename,
// 		key:  []byte(path),
// 		st:   nil_storage{},
// 		en:   c4.NewEncoder(),
// 	}
// }

// TODO: if a.id == nil try to find the ID in the C4 db.  If the asset is open, then
// return the Encoder.ID() of the asset. If the ID is nil, the file is not open, and there
// is no id in the database, then open the file and identify it.
func (a *file_asset) ID() *c4.ID {
	if a.id != nil {
		return a.id
	}
	return a.en.ID()
}

func (a *file_asset) Storage() abstract_storage_interface {
	return a.st
}

func (a *folder_asset) Close() error {
	if a.mode == os.O_RDONLY {
		if a.f != nil {
			return a.f.Close()
		}
	}
	return nil
}

func (a *file_asset) Close() error {
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
func (a *file_asset) commit() error {
	// Close the file
	err := a.f.Close()
	if err != nil {
		return err
	}
	// fmt.Printf("commit: %s\n", a.key)
	id := a.en.ID()
	if id == nil {
		return noIdError(string(a.key))
	}
	// fmt.Printf("commit: id: %s\n", id)

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
func (a *file_asset) Name() string {
	return a.name
}

// Read reads up to len(b) bytes from the Asset. It returns the number of bytes read and
// any error encountered. At end of file, Read returns 0, io.EOF.
func (a *file_asset) Read(b []byte) (n int, err error) {
	return a.f.Read(b)
}

// ReadAt reads len(b) bytes from the Asset starting at byte offset off. It returns the
// number of bytes read and the error, if any. ReadAt always returns a non-nil error when
// n < len(b). At end of file, that error is io.EOF.
func (a *file_asset) ReadAt(b []byte, off int64) (n int, err error) {
	return a.f.ReadAt(b, off)
}

// Seek sets the offset for the next Read or Write on file to offset, interpreted
// according to whence: 0 means relative to the origin of the file, 1 means relative to
// the current offset, and 2 means relative to the end. It returns the new offset and an
// error, if any.
func (a *file_asset) Seek(offset int64, whence int) (ret int64, err error) {
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
func (a *file_asset) Readdir(n int) (files []os.FileInfo, err error) {
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
func (a *file_asset) Readdirnames(n int) (names []string, err error) {
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

// Write writes len(b) bytes to the Asset. It returns the number of bytes written and an
// error, if any. Write returns a non-nil error when n != len(b).
func (a *file_asset) Write(b []byte) (n int, err error) {
	w := io.MultiWriter(a.en, a.f)
	n64, er := io.Copy(w, bytes.NewReader(b))
	return int(n64), er
}

// WriteAt writes len(b) bytes to the File starting at byte offset off. It returns the
// number of bytes written and an error, if any. WriteAt returns a non-nil error when n
// != len(b).
func (a *file_asset) WriteAt(b []byte, off int64) (n int, err error) {
	// TODO: WriteAt cannot ID with MultiWriter
	return a.f.WriteAt(b, off)
}

func (a *file_asset) WriteString(s string) (n int, err error) {
	return a.Write([]byte(s))
}

func (a *file_asset) Stat() (info os.FileInfo, err error) {
	err = errors.New("unimplemented")
	return
}

func (a *file_asset) Remove() {
	if a.f == nil {
		return
	}
	a.f.Close()
	os.Remove(a.f.Name())
}

// asset_copy is a convenience function that returns a shallow copy of an asset.
func file_asset_copy(a file_asset) Asset {
	return &file_asset{
		name: a.name,
		key:  a.key,
		mode: a.mode,
		f:    a.f,
		st:   a.st,
		en:   a.en,
		id:   a.id,
	}
}
