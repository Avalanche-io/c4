package store

import c4 "github.com/avalanche-io/c4/id"

// abstract_storage_interface provides an interface to abstract the underlying storage
// of an asset.
type abstract_storage_interface interface {
	move(path string, id *c4.ID) error
	set(key []byte, id *c4.ID) error
	updateDirectory(key []byte) error
}

type storage Store
type nil_storage struct{}

func (s *storage) move(path string, id *c4.ID) error {
	return movetoid(s.path, path, id)
}

func (s *storage) set(key []byte, id *c4.ID) error {
	return (*Store)(s).db.Set(key, id)
}

func (s *storage) updateDirectory(key []byte) error {
	return (*Store)(s).update_directory(key)
}

func (nil_storage) move(path string, id *c4.ID) (err error) {
	return nil
}

func (nil_storage) set(key []byte, id *c4.ID) error {
	return nil
}

func (nil_storage) updateDirectory(key []byte) error {
	return nil
}
