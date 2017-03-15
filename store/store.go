package store

import (
	"io"
	"os"
	"path/filepath"
	assetpath "path/filepath"

	c4db "github.com/Avalanche-io/c4/db"
	c4 "github.com/Avalanche-io/c4/id"
	slash "github.com/Avalanche-io/path"
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
	if !s.Exists("/") {
		err = makeroot(path, db)
	}
	return s, err
}

// Create creates a new writable asset.
func (s *Store) Create(path string) (Asset, error) {
	temp_file, err := tmp(s.path)
	if err != nil {
		return nil, err
	}
	return NewFileAsset(path, (*storage)(s), os.O_RDWR, temp_file)
}

//Open opens the named asset for reading.
func (s *Store) Open(name string) (a Asset, err error) {
	id := s.db.GetAssetID([]byte(name))
	if id == nil {
		return nil, os.ErrNotExist
	}
	var file *os.File
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
	if slash.IsDir(name) {
		return NewDirAsset(name, (*storage)(s), os.O_RDONLY, file)
	}
	return NewFileAsset(name, (*storage)(s), os.O_RDONLY, file)
}

// Mkdir creates an empty directory entry for the given path if it does not already
// exist. Mkdir returns os.ErrExist if the path already exists.
func (s *Store) Mkdir(path string) error {
	if path == "/" {
		return nil
	}

	if path[len(path)-1:] != "/" {
		return dirError("directory must have \"/\" suffix")
	}
	if s.Exists(path) {
		return os.ErrExist
	}
	err := s.db.SetAssetID([]byte(path), c4.NIL_ID)
	if err != nil {
		return err
	}
	return s.update_directory([]byte(path))
}

// MkdirAll makes directories in the given bath that don't already exist.
func (s *Store) MkdirAll(path string) error {
	if path[len(path)-1:] != "/" {
		return dirError("directory must have \"/\" suffix")
	}
	p, err := slash.New(path)
	if err != nil {
		return err
	}
	for _, dir := range p.EveryPath() {
		if !s.Exists(dir) {
			err = s.Mkdir(dir)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// AssetID returns the c4 id of file at the given path
func (s *Store) AssetID(name string) *c4.ID {
	return s.db.GetAssetID([]byte(name))
}

// Close closes the database, and any other cleanup as needed.
func (s *Store) Close() error {
	return s.db.Close()
}

// Exists tests if the path exists in the database, and if the identified file
// exists in the storage.
func (s *Store) Exists(path string) bool {
	id := s.db.GetAssetID([]byte(path))
	if id == nil {
		return false
	}
	e := exists(s.path, id)
	return e
}

// Add returns a copy of the Asset bound to the storage, or the unmodified Asset if it
// is already bound.
func (s *Store) Add(asset Asset) Asset {
	switch val := asset.Storage().(type) {
	case *storage:
		if s == (*Store)(val) {
			return asset
		}
	}

	return CopyAsset(asset, (*storage)(s))
}

// update_directory adds the file name in path to it's parent directory list and saves
func (s *Store) update_directory(key []byte) error {
	dir, name := assetpath.Split(string(key))
	if slash.IsDir(string(key)) {
		p, err := slash.New(string(key) + "/")
		if err != nil {
			return err
		}
		dir, name = p.Split()
	}

	var d Directory
	din, err := s.Open(dir)
	if err != nil {
		return err
	}
	defer din.Close()

	// read the file
	_, err = io.Copy(&d, din)
	if err != nil {
		return dirError("unable to read directory \"" + dir + "\" \"" + name + "\"" + err.Error())
	}

	// UPDATE
	// add the name to the directory
	d.Insert(name)

	// WRITE
	// create a new file
	dout, err := s.Create(dir)
	if err != nil {
		return dirError(err.Error())
	}

	// write data from the directory in ram
	_, err = io.Copy(dout, d)
	if err != nil {
		dout.Remove()
		return dirError(err.Error())
	}
	// commit changes.
	return dout.commit()
}
