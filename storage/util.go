package store

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Avalanche-io/c4/db"
	c4 "github.com/Avalanche-io/c4/id"
)

// updated, delete me

// makepaths makes directories on the real filesystem
func makepaths(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

// pathtoasset returns the directory path to an asset.
// It creates a few sub directories to keep the top level directory from
// getting to big.
func pathtoasset(path string, id *c4.ID) string {
	str := id.String()
	newpath := path
	for i := 0; i < 2; i++ {
		newpath = filepath.Join(newpath, str[i*2:i*2+2])
	}
	return newpath
}

// movetoid renames a file to the c4 id provided, using pathtoasset structure.
func movetoid(path, source string, id *c4.ID) error {
	// if an the file already exists then exit early.
	if exists(path, id) {
		return nil
	}
	target := filepath.Join(pathtoasset(path, id), id.String())
	// make any needed directories
	dir, _ := filepath.Split(target)
	makepaths(dir)
	// rename the source to the target
	return os.Rename(source, target)
}

// exists tests if a file for the given id already exits in the storage at path.
func exists(path string, id *c4.ID) bool {
	idpath := filepath.Join(pathtoasset(path, id), id.String())
	if _, err := os.Stat(idpath); os.IsNotExist(err) {
		return false
	}
	return true
}

func makeroot(path string, db *db.DB) error {
	tmp := filepath.Join(path, writepath)
	temp_file, err := ioutil.TempFile(tmp, "")
	if err != nil {
		return err
	}
	temp_file.Close()
	movetoid(path, temp_file.Name(), c4.NIL_ID)
	_, err = db.KeySet("/", c4.NIL_ID.Digest())
	return err
}

func tmp(path string) (*os.File, error) {
	tmp := filepath.Join(path, writepath)
	temp_file, err := ioutil.TempFile(tmp, "")
	if err != nil {
		return nil, err
	}
	return temp_file, nil
}
