package db

import (
	"bytes"
	"os"

	c4 "github.com/avalanche-io/c4/id"
	"github.com/boltdb/bolt"
)

type DB bolt.DB

type Options bolt.Options

type BucketType uint

const (
	Asset BucketType = iota
	Attribute
	IDs
)

var (
	AsssetBucket     []byte = []byte("assets")
	AttributesBucket []byte = []byte("attributes")
	IDBucket         []byte = []byte("ids")
)

func Open(path string, mode os.FileMode, options *Options) (*DB, error) {
	bdb, err := bolt.Open(path, mode, (*bolt.Options)(options))
	if err != nil {
		return nil, err
	}
	err = bdb.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(AsssetBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(AttributesBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(IDBucket)
		return err
	})

	return (*DB)(bdb), err
}

func (d *DB) getIDFromBucket(key []byte, bkt BucketType) *c4.ID {
	bucket := AsssetBucket
	if bkt == Attribute {
		bucket = AttributesBucket
	}
	var id *c4.ID
	(*bolt.DB)(d).View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucket).Get(key)
		if data != nil {
			digest := make([]byte, 64)
			copy(digest, data)
			id = c4.Digest(digest).ID()
		}
		return nil
	})
	return id
}

func (d *DB) setIDForBucket(key []byte, id *c4.ID, bkt BucketType) error {
	bucket := AsssetBucket
	if bkt == Attribute {
		bucket = AttributesBucket
	}
	newid := id.Digest()
	return (*bolt.DB)(d).Batch(func(tx *bolt.Tx) error {
		oldid := tx.Bucket(bucket).Get(key)
		if bytes.Compare(oldid, newid) == 0 {
			return nil
		}
		if oldid != nil {
			oldcount := tx.Bucket(IDBucket).Get(oldid)
			oldcount2 := make([]byte, len(oldcount))
			copy(oldcount2, oldcount)
			oldcount2 = decr(oldcount2)
			if len(oldcount2) == 1 && oldcount2[0] == 0 { // if oldcount is 0
				tx.Bucket(IDBucket).Delete(oldid)
			} else {
				err := tx.Bucket(IDBucket).Put(oldid, oldcount2) // save the decremented oldcount
				if err != nil {
					return err
				}
			}
		}
		err := tx.Bucket(bucket).Put(key, newid)
		if err != nil {
			return err
		}
		newcount := tx.Bucket(IDBucket).Get(newid)
		if newcount == nil {
			newcount = []byte{0}
		}
		newcount2 := make([]byte, len(newcount), len(newcount)+1)
		copy(newcount2, newcount)
		newcount2 = incr(newcount2)
		return tx.Bucket(IDBucket).Put(newid, newcount2)
	})
}

func (d *DB) Unset(key []byte) *c4.ID {
	return d.unsetBucket(key, Asset)
}

func (d *DB) UnsetAttributes(key []byte) *c4.ID {
	return d.unsetBucket(key, Attribute)
}

func (d *DB) unsetBucket(key []byte, bkt BucketType) *c4.ID {
	var id *c4.ID
	bucket := AsssetBucket
	if bkt == Attribute {
		bucket = AttributesBucket
	}
	(*bolt.DB)(d).Batch(func(tx *bolt.Tx) error {
		// Get the value of key
		oldid := tx.Bucket(bucket).Get(key)
		if oldid != nil {
			digest := make([]byte, 64)
			copy(digest, oldid)
			id = c4.Digest(digest).ID()
			oldcount := tx.Bucket(IDBucket).Get(oldid)
			oldcount2 := make([]byte, len(oldcount))
			copy(oldcount2, oldcount)
			oldcount2 = decr(oldcount2)
			if len(oldcount2) == 1 && oldcount2[0] == 0 { // if oldcount2 is 0
				tx.Bucket(IDBucket).Delete(oldid)
			} else {
				err := tx.Bucket(IDBucket).Put(oldid, oldcount2) // save the decremented oldcount
				if err != nil {
					return err
				}
			}
		}
		return tx.Bucket(bucket).Delete(key)
	})
	return id
}

func (d *DB) IDexists(id *c4.ID) bool {
	res := true
	(*bolt.DB)(d).View(func(tx *bolt.Tx) error {
		c := tx.Bucket(IDBucket).Get(id.Digest())
		if c == nil {
			res = false
		}
		return nil
	})
	return res
}

// incr2 48x faster than using big.Int, 8x slower then uint64
func incr(data []byte) []byte {
	m := 0x100
	for i := len(data) - 1; i >= 0; i-- {
		m = 1 + int(data[i])
		data[i] = byte(m)
		if m^0x100 != 0 { // if we don't overflow the byte then we're done
			return data
		}
	}
	// we've run out of allocated bytes so we need another one.
	data = make([]byte, len(data)+1)
	data[0] = 0x01
	return data
}

func decr(data []byte) []byte {
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] != 0 {
			data[i]--
			break
		}
		if i == 0 {
			return []byte{0}
		}
		data[i] = 0xff
	}
	if len(data) > 1 && data[0] == 0 {
		return data[1:]
	}
	return data
}

func (db *DB) Set(key []byte, id *c4.ID) error {
	return db.setIDForBucket(key, id, Asset)
}

func (db *DB) Get(key []byte) *c4.ID {
	return db.getIDFromBucket(key, Asset)
}

func (db *DB) SetAttributes(key []byte, id *c4.ID) error {
	return db.setIDForBucket(key, id, Attribute)
}

func (db *DB) GetAttributes(key []byte) *c4.ID {
	return db.getIDFromBucket(key, Attribute)
}

func (d *DB) ForEach(f func(key []byte, asset *c4.ID, attributes *c4.ID) error) {
	db := (*bolt.DB)(d)
	db.View(func(tx *bolt.Tx) error {
		assets_bkt := tx.Bucket(AsssetBucket)
		attributes_bkt := tx.Bucket(AttributesBucket)
		return assets_bkt.ForEach(func(key []byte, asset_value []byte) error {
			if len(asset_value) != 64 {
				return nil
			}
			attribute_value := attributes_bkt.Get(key)
			if len(attribute_value) != 64 {
				return nil
			}
			asset_id := c4.NewDigest(asset_value).ID()
			attribute_id := c4.NewDigest(attribute_value).ID()
			return f(key, asset_id, attribute_id)
		})
	})
}

func (d *DB) Close() error {
	db := (*bolt.DB)(d)
	return db.Close()
}
