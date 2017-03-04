package store

import (
	"errors"
	"io/ioutil"
	"os"
	assetpath "path"
	"path/filepath"
	"sort"
	"strings"

	c4db "github.com/Avalanche-io/c4/db"
	c4id "github.com/Avalanche-io/c4/id"
)

// Store represents a Asset storage location.
type Store struct {
	path string
	db   *c4db.DB
}

// writepath represents a writable folder for files prior to identification.
const writepath string = "scratch"

// OpenStorage opens the storage at the given path.  If the path doesn't already
// exist, OpenStorage will attempt to create it.
func Open(path string) (*Store, error) {
	// Make paths as necessary.
	err := makepaths(path, filepath.Join(path, writepath))
	if err != nil {
		return nil, err
	}

	// Open a C4 Database
	db_path := filepath.Join(path, "c4id.db")
	db, err := c4db.Open(db_path, 0600, nil)
	if err != nil {
		return nil, err
	}

	// initialize and return a new Store
	s := &Store{path, db}
	err = s.makeroot()
	return s, err
}

func (s *Store) create_temp() (*os.File, error) {
	tmp := filepath.Join(s.path, writepath)
	return ioutil.TempFile(tmp, "")
}

// Create creates a new writable asset.
func (s *Store) Create(path string) (*Asset, error) {
	temp_file, err := s.create_temp()
	if err != nil {
		return nil, err
	}
	a := NewAsset(path)
	a.st = (*storage)(s)
	a.mode = os.O_RDWR
	a.f = temp_file
	return a, nil
}

func (s *Store) makeroot() error {
	if !s.Exists("/") {
		return s.Mkdir("/")
	}
	return nil
}

func (s *Store) AssetID(name string) *c4id.ID {
	return s.db.GetAssetID([]byte(name))
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

	err = a.st.updateDirectory([]byte(name))
	if err != nil {
		return err
	}
	return nil

	return err
}

func (s *Store) MkdirAll(path string) (err error) {
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

func exists(path string, id *c4id.ID) bool {
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
	var en *c4id.Encoder
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
		st:   (*storage)(s),
		id:   id,
		en:   en,
	}
	return a, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Add returns a copy of the Asset bound to the storage, or the unmodified *Asset if it
// is already bound.
func (s *Store) Add(asset *Asset) *Asset {
	switch val := asset.st.(type) {
	case *storage:
		if s == (*Store)(val) {
			return asset
		}
	}

	a := asset_copy(asset)
	a.st = (*storage)(s)
	return a
}

func (s *Store) OpenAsset(name string, flag int, perm os.FileMode) (a *Asset, err error) {
	err = errors.New("unimplemented")
	return
}

func (s *Store) movetoid(path string, id *c4id.ID) error {
	idpath := filepath.Join(pathtoasset(s.path, id), id.String())
	if !exists(s.path, id) {
		dir, _ := filepath.Split(idpath)
		makepaths(dir)
		err := os.Rename(path, idpath)
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

// update_directory adds the file name in path to it's parent directory list and saves
func (s *Store) update_directory(key []byte) error {
	var suffix string
	if string(key[len(key)-1:]) == "/" {
		suffix = "/"
		key = key[:len(key)-1]
	}
	dir, name := assetpath.Split(string(key))
	if len(name) == 0 {
		dir, name = assetpath.Split(dir)
		if len(dir) == 1 {
			name = "/"
			dir = ""
		}
	}
	name += suffix

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
		return s.write_dirnames(d2, names)
	}
	if names[i] == name {
		return nil
	}
	// insert
	names = append(names, "")
	copy(names[i+1:], names[i:])
	names[i] = name
	return s.write_dirnames(d2, names)
}

func (s *Store) write_dirnames(a *Asset, names []string) error {
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
	return a.commit()
}

func makepaths(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

func pathtoasset(path string, id *c4id.ID) string {
	str := id.String()

	newpath := path

	for i := 0; i < 8; i++ {
		newpath = filepath.Join(newpath, str[i*2:i*2+2])
	}

	return newpath
}
