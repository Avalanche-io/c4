package db

import (
	"os"

	c4 "github.com/Avalanche-io/c4/id"
	"github.com/boltdb/bolt"
)

type DB bolt.DB

type Options bolt.Options

func Open(path string, mode os.FileMode, options *Options) (*DB, error) {
	bdb, err := bolt.Open(path, mode, (*bolt.Options)(options))
	if err != nil {
		return nil, err
	}
	err = bdb.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("assets")); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte("attributes")); err != nil {
			return err
		}
		return nil
	})

	return (*DB)(bdb), err
}

func (d *DB) getIDFromBucket(key []byte, bucket string) *c4.ID {
	var id *c4.ID
	db := (*bolt.DB)(d)
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		idbytes := b.Get(key)
		if len(idbytes) == 64 {
			id = c4.NewDigest(idbytes).ID()
		}
		return nil
	})
	return id
}

func (d *DB) setIDForBucket(key []byte, id *c4.ID, bucket string) error {
	db := (*bolt.DB)(d)
	return db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		return b.Put(key, id.Digest())
		return nil
	})
}

func (db *DB) SetAssetID(key []byte, id *c4.ID) error {
	return db.setIDForBucket(key, id, "assets")
}

func (db *DB) SetAttributesID(key []byte, id *c4.ID) error {
	return db.setIDForBucket(key, id, "attributes")
}

func (db *DB) GetAssetID(key []byte) *c4.ID {
	return db.getIDFromBucket(key, "assets")
}

func (db *DB) GetAttributesID(key []byte) *c4.ID {
	return db.getIDFromBucket(key, "attributes")
}

func (d *DB) ForEach(f func(key []byte, asset *c4.ID, attributes *c4.ID) error) {
	db := (*bolt.DB)(d)
	db.View(func(tx *bolt.Tx) error {
		assets_bkt := tx.Bucket([]byte([]byte("assets")))
		attributes_bkt := tx.Bucket([]byte([]byte("attributes")))
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
