package store

import (
	"bytes"
	"encoding/json"
	// "fmt"
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
	db_path := filepath.Join(path, "c4.db")
	db, err := c4db.Open(db_path, nil)
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
func (s *Store) Create(path string, ids ...*c4.ID) (Asset, error) {
	var id *c4.ID
	if len(ids) == 1 {
		id = ids[0]
	}

	temp_file, err := tmp(s.path)
	if err != nil {
		return nil, err
	}
	return NewFileAsset(path, (*storage)(s), os.O_RDWR, temp_file, id)
}

func (s *Store) Writer(path string, ids ...*c4.ID) (c4.WriteCloser, error) {
	return s.Create(path, ids...)
}

func (s *Store) Reader(path string, ids ...*c4.ID) (c4.ReadCloser, error) {
	return s.Open(path, ids...)
}

func (s *Store) Copy(src, dest string) error {
	digest, err := s.db.KeyGet(src)
	if err != nil {
		return err
	}
	if digest == nil {
		return ErrNotFound
	}
	_, err = s.db.KeySet(dest, digest)
	return err
}

func (s *Store) Move(src, dest string) error {
	err := s.Copy(src, dest)
	if err != nil {
		return err
	}
	_, err = s.db.KeyDelete(src)
	return err
}

//Open opens the named asset for reading.
func (s *Store) Open(name string, ids ...*c4.ID) (a Asset, err error) {
	var id *c4.ID
	if len(ids) == 1 {
		id = ids[0]
	}
	if len(name) > 0 {
		var digest c4.Digest
		digest, err = s.db.KeyGet(name)
		if digest != nil {
			id = digest.ID()
		}
	}

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
		dir, _ = filepath.Split(dir)
		if len(dir) == 1 {
			filename = "/"
			dir = ""
		}
	}
	if slash.IsDir(name) {
		return NewDirAsset(name, (*storage)(s), os.O_RDONLY, file)
	}
	return NewFileAsset(name, (*storage)(s), os.O_RDONLY, file, id)
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
	_, err := s.db.KeySet(path, c4.NIL_ID.Digest())
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
	d, err := s.db.KeyGet(name)
	if err != nil {
		return nil
	}
	return d.ID()
}

// Close closes the database, and any other cleanup as needed.
func (s *Store) Close() error {
	return s.db.Close()
}

// Exists tests if the path exists in the database, and if the identified file
// exists in the storage.
func (s *Store) Exists(path string) bool {
	digest, err := s.db.KeyGet(path)
	if err != nil {
		return false
	}
	if digest == nil {
		return false
	}
	e := exists(s.path, digest.ID())
	return e
}

func (s *Store) IDexists(id *c4.ID) bool {
	key := s.db.KeyFind(id.Digest())
	if len(key) == 0 {
		return false
	}
	return true
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

func (s *Store) SetAttributes(key string, attrs map[string]interface{}) error {
	// convert to json
	data, err := json.Marshal(attrs)
	if err != nil {
		return err
	}
	// identify
	id := c4.Identify(bytes.NewReader(data))

	// Check if the id already exists.
	if exists(s.path, id) {
		asset_digest, err := s.db.KeyGet(key)
		if err != nil {
			return err
		}
		s.db.LinkSet("attributes", asset_digest, id.Digest())
		// s.db.SetAttributes([]byte(key), id)
		return nil
	}
	dir := pathtoasset(s.path, id)
	makepaths(dir)
	file_name := filepath.Join(dir, id.String())

	f, err := os.Create(file_name)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, bytes.NewReader(data))
	if err != nil {
		return err
	}
	_ = n
	f.Close()
	asset_digest, err := s.db.KeyGet(key)
	if err != nil {
		return err
	}
	s.db.LinkSet("attributes", asset_digest, id.Digest())

	// s.db.SetAttributes([]byte(key), id)
	return nil
}

func (s *Store) GetAttributes(key string, attrs map[string]interface{}) error {
	asset_digest, err := s.db.KeyGet(key)
	if err != nil {
		return err
	}
	var id *c4.ID
	for ent := range s.db.LinkGet("attributes", asset_digest) {
		digest := ent.Target()
		if digest != nil {
			id = digest.ID()
		}
		ent.Close()
	}

	// id := s.db.GetAttributes([]byte(key))
	if id == nil {
		return ErrNotFound
	}
	file_name := filepath.Join(pathtoasset(s.path, id), id.String())
	f, err := os.Open(file_name)
	if err != nil {
		return err
	}
	defer f.Close()
	j := json.NewDecoder(f)
	err = j.Decode(&attrs)
	if err != nil {
		return err
	}
	return nil
}

// update_directory adds the file name in path to it's parent directory
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
