package db

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	c4 "github.com/Avalanche-io/c4/id"
)

// Store represents a Asset storage location.
type Store struct {
	path string
	db   *DB
}

// Asset represents
type Asset struct {
	name string
	key  []byte
	mode int
	f    *os.File
	st   *Store
	en   *c4.IDEncoder
	id   *c4.ID
}

// OpenStorage opens the storage at the given path.  If the path doesn't already
// exist, OpenStorage will attempt to create it.
func OpenStorage(path string) (*Store, error) {
	temp_path := filepath.Join(path, "temp")
	err := makepaths(path, temp_path)
	if err != nil {
		return nil, err
	}

	db_path := filepath.Join(path, "c4.db")
	db, err := Open(db_path, 0600, nil)
	if err != nil {
		return nil, err
	}
	s := &Store{path, db}
	err = s.makeroot()
	if err != nil {
		return nil, err
	}
	return s, nil
}

/*
Create creates the named asset, replacing the file if the file already exists. If
successful, methods on the returned File can be used for I/O; the
associated asset descriptor has mode O_RDWR.
*/
func (s *Store) Create(name string) (a *Asset, err error) {
	// If there is an ID for this name the
	tmp := filepath.Join(s.path, "temp")
	var file *os.File
	file, err = ioutil.TempFile(tmp, "")
	if err != nil {
		return nil, err
	}
	en := c4.NewIDEncoder(file)
	_, filename := filepath.Split(name)
	key := []byte(name)
	a = &Asset{
		name: filename,
		key:  key,
		mode: os.O_RDWR,
		f:    file,
		st:   s,
		id:   nil,
		en:   en,
	}
	return a, nil
}

type directory []string

func (s *Store) makeroot() error {
	if !s.Exists("/") {
		return s.Mkdir("/")
	}
	return nil
}

type mkdirError string

func (e mkdirError) Error() string {
	return "mkdir error: " + string(e)
}

func (s *Store) Mkdir(name string) error {
	if name[len(name)-1:] != "/" {
		name += "/"
	}
	if s.Exists(name) {
		return os.ErrExist
	}
	a, err := s.Create(name)
	if err != nil {
		return mkdirError(err.Error())
	}
	err = a.f.Close()
	if err != nil {
		return err
	}

	id := a.en.ID()
	err = s.movetoid(a.f.Name(), id)
	if err != nil {
		return err
	}
	err = s.db.SetAssetID([]byte(name), id)
	if err != nil {
		return err
	}

	if name == "/" {
		return nil
	}

	folders := strings.Split(name[:len(name)-1], "/")
	dir := strings.Join(folders[:len(folders)-1], "/") + "/"
	foldername := folders[len(folders)-1] + "/"

	err = a.st.update_directory(dir, foldername)
	if err != nil {
		return err
	}
	return nil

	return err
}

func (s *Store) AssetID(name string) *c4.ID {
	return s.db.GetAssetID([]byte(name))
}

func (s *Store) MkdirAll(path string) (err error) {
	// c4 uses "/" on all systems, so we should avoid filepath.Seperator
	folders := strings.Split(path, "/")
	cwd := "/"
	for i, dir := range folders {
		if len(dir) == 0 {
			continue
		}
		_ = i
		cwd = cwd + dir
		if !s.Exists(cwd) {
			err = s.Mkdir(cwd)
			if err != nil {
				return
			}
		}
		cwd += "/"
	}
	return
}

func (s *Store) Exists(path string) bool {
	id := s.db.GetAssetID([]byte(path))
	if id == nil {
		return false
	}
	e := exists(s.path, id)
	return e
}

func exists(path string, id *c4.ID) bool {
	idpath := filepath.Join(pathtoasset(path, id), id.String())
	if _, err := os.Stat(idpath); os.IsNotExist(err) {
		return false
	}
	return true
}

//Open opens the named asset for reading.
func (s *Store) Open(name string) (a *Asset, err error) {
	id := s.db.GetAssetID([]byte(name))
	if id == nil {
		return nil, os.ErrNotExist
	}
	var file *os.File
	var en *c4.IDEncoder
	file_name := filepath.Join(pathtoasset(s.path, id), id.String())
	file, err = os.OpenFile(file_name, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}

	dir, filename := filepath.Split(name)
	if len(filename) == 0 {
		dir, filename = filepath.Split(dir)
		if len(dir) == 1 {
			filename = "/"
			dir = ""
		}
	}

	key := []byte(name)
	a = &Asset{
		name: filename,
		key:  key,
		mode: os.O_RDONLY,
		f:    file,
		st:   s,
		id:   id,
		en:   en,
	}
	return a, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) OpenAsset(name string, flag int, perm os.FileMode) (a *Asset, err error) {
	err = errors.New("unimplemented")
	return
}

func (a *Asset) ID() *c4.ID {
	return a.id
}

func (s *Store) movetoid(path string, id *c4.ID) error {
	idpath := filepath.Join(pathtoasset(s.path, id), id.String())
	if !exists(s.path, id) {
		dir, _ := filepath.Split(idpath)
		makepaths(dir)
		err := os.Rename(path, idpath)
		return err
	}
	return nil
}

type dirError string

func (e dirError) Error() string {
	return "directory error: " + string(e)
}

func (a *Asset) Close() error {
	if a.mode == os.O_RDONLY {
		return a.f.Close()
	}

	err := a.f.Close()
	if err != nil || a.id != nil {
		a.st.db.SetAssetID(a.key, a.id)
		return err
	}
	a.id = a.en.ID()
	err = a.st.movetoid(a.f.Name(), a.id)
	if err != nil {
		return err
	}

	err = a.st.db.SetAssetID(a.key, a.id)
	if err != nil {
		return err
	}
	path := string(a.key)
	// FIX: don't use filepath (filepath.Seperator with change per OS)
	dir, filename := filepath.Split(path)
	if len(filename) == 0 {
		dir, filename = filepath.Split(dir)
		if len(dir) == 1 {
			filename = "/"
			dir = ""
		}
	}
	err = a.st.update_directory(dir, filename)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) open_directory(path string) (*Asset, error) {
	if !s.Exists(path) {
		return nil, dirError("open_directory \"" + path + "\" directory does not exist")
	}

	d, err := s.Open(path)
	if err != nil {
		return nil, dirError("open_directory \"" + path + "\" " + err.Error())
	}
	return d, err
}

// update_directory adds the file name in path to it's parent directories
// list and saves the new list.
func (s *Store) update_directory(dir, name string) error {
	d, err := s.open_directory(dir)
	if err != nil {
		return err
	}
	names, err := d.Readdirnames(-1)
	if err != nil {
		return dirError("update_directory \"" + dir + "\" \"" + name + "\"" + err.Error())
	}

	err = d.Close()
	if err != nil {
		return dirError("update_directory \"" + dir + "\" \"" + name + "\"" + err.Error())
	}

	d2, err := s.Create(dir)
	if err != nil {
		return dirError(err.Error())
	}
	i := sort.SearchStrings(names, name)
	if i == len(names) {
		names = append(names, name)
		err := d2.write_dirnames(names)
		if err != nil {
			return dirError(err.Error())
		}
		return nil
	}
	if names[i] == name {
		return nil
	}
	// insert
	names = append(names, "")
	copy(names[i+1:], names[i:])
	names[i] = name
	err = d2.write_dirnames(names)
	if err != nil {
		return dirError(err.Error())
	}
	return nil
}

func (a *Asset) write_dirnames(names []string) error {

	for i, name := range names {
		b := []byte(name)
		if i < len(names)-1 {
			b = append(b, byte(0))
		}
		_, err := a.Write(b)
		if err != nil {
			return err
		}
	}
	err := a.f.Close()
	a.id = a.en.ID()
	err = a.st.movetoid(a.f.Name(), a.id)
	if err != nil {
		return err
	}

	err = a.st.db.SetAssetID(a.key, a.id)
	if err != nil {
		return err
	}

	return nil
}

func (a *Asset) Name() string {
	return a.name
}

func (a *Asset) Read(b []byte) (n int, err error) {
	return a.f.Read(b)
}

func (a *Asset) ReadAt(b []byte, off int64) (n int, err error) {
	return a.f.ReadAt(b, off)
}

func (a *Asset) Seek(offset int64, whence int) (ret int64, err error) {
	return a.f.Seek(offset, whence)
}

// Readdir reads the contents of the directory associated with file and returns
// a slice of up to n FileInfo values, as would be returned by Lstat, in
// directory order. Subsequent calls on the same file will yield further
// FileInfos.

// If n > 0, Readdir returns at most n FileInfo structures. In this case, if
// Readdir returns an empty slice, it will return a non-nil error explaining
// why.  At the end of a directory, the error is io.EOF.

// If n <= 0, Readdir returns all the FileInfo from the directory in a single
// slice. In this case, if Readdir succeeds (reads all the way to the end of the
// directory), it returns the slice and a nil error. If it encounters an error
// before the end of the directory, Readdir returns the FileInfo read until that
// point and a non-nil error.
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

var ErrDirUnderflow error = errors.New("string of length 0 in directory list")

// directoryscanner implements bufio.SplitFunc.  It tokenizes byte(0) delimited
// strings.  And error is returned if a string of length 0 is parsed (i.e. two
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

func (a *Asset) Write(b []byte) (n int, err error) {
	n64, er := io.Copy(a.en, bytes.NewReader(b))
	return int(n64), er
}

func (a *Asset) WriteAt(b []byte, off int64) (n int, err error) {
	err = errors.New("unimplemented")
	return
}

func (a *Asset) WriteString(s string) (n int, err error) {
	err = errors.New("unimplemented")
	return
}

func (a *Asset) Stat() (info os.FileInfo, err error) {
	err = errors.New("unimplemented")
	return
}

func makepaths(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

func pathtoasset(path string, id *c4.ID) string {
	str := id.String()

	newpath := path

	for i := 0; i < 8; i++ {
		newpath = filepath.Join(newpath, str[i*2:i*2+2])
	}

	return newpath
}
