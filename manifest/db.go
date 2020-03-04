package manifest

import (
	bolt "go.etcd.io/bbolt"
)

type MDB struct {
	Db *bolt.DB

	// Path to c4 labeled storage of manifests
	storage string
}

func NewDb(db *bolt.DB, storagepath string) *MDB {
	return &MDB{db, storagepath}
}
