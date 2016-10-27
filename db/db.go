// The c4 db package provide Key/Value memory types, db interface, and
// buffers for multiprocessing.
package db

import (
	// "bytes"
	"encoding/json"
	"errors"
	"log"

	"github.com/boltdb/bolt"
)

type DB bolt.DB

type Element interface {
	Key() string
	Value() interface{}
}

type DBE struct {
	key   string
	value interface{}
}

func (k *DBE) Key() string {
	return k.key
}

func (k *DBE) Value() interface{} {
	return k.value
}

// Open creates or opens the given db path with permissions 0600.
func Open(path string) (*DB, error) {
	db, err := bolt.Open(path, 0600, nil)
	return (*DB)(db), err
}

// Close closes a the database file.
func (db *DB) Close() {
	(*bolt.DB)(db).Close()
}

type createBucketError string

func (e createBucketError) Error() string {
	return "Error creating bucket: " + string(e)
}

// CreateBuckets creates the given array of bucket.
func (db *DB) CreateBuckets(names []string) error {
	bdb := (*bolt.DB)(db)

	return bdb.Update(func(tx *bolt.Tx) error {
		for _, name := range names {
			_, err := tx.CreateBucketIfNotExists([]byte(name))
			if err != nil {
				return createBucketError(name + err.Error())
			}
		}
		return nil
	})
}

// CreateBucket creates a single bucket
func (db *DB) CreateBucket(name string) error {
	bdb := (*bolt.DB)(db)

	return bdb.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return createBucketError(name + err.Error())
		}
		return nil
	})
}

// List buckets returns a list of buckets as an array of strings
func (db *DB) ListBuckets() ([]string, error) {
	bdb := (*bolt.DB)(db)

	var result []string
	err := bdb.View(func(tx *bolt.Tx) error {
		err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			result = append(result, string(name))
			return nil
		})
		return err
	})
	return result, err
}

// Put sets the value of a key for a given bucket
func (db *DB) Put(bucket string, key []byte, data []byte) error {
	bdb := (*bolt.DB)(db)

	return bdb.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		return b.Put(key, data)
	})
}

// Get retrieves the value a key for the given bucket
func (db *DB) Get(bucket string, key []byte) ([]byte, error) {
	bdb := (*bolt.DB)(db)

	var data []byte
	err := bdb.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		data = b.Get(key)
		return nil
	})
	return data, err
}

// Iterate over keys
func (db *DB) Iterate(bucket string, f func(k []byte, v []byte) bool) error {
	bdb := (*bolt.DB)(db)

	err := bdb.View(func(tx *bolt.Tx) error {
		// Assume bucket exists and has keys
		b := tx.Bucket([]byte(bucket))
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			if !f(k, v) {
				break
			}
		}
		return nil
	})
	return err
}

func (db *DB) Iterator(bucket string, key []byte, cancel <-chan struct{}) <-chan Element {
	bdb := (*bolt.DB)(db)

	out := make(chan Element)
	// if key == nil {
	// 	log.Println("key: NIL")

	// } else {
	// 	log.Printf("key: %s\n", string(key))
	// }
	go func() {
		err := bdb.View(func(tx *bolt.Tx) error {
			// Assume bucket exists and has keys
			b := tx.Bucket([]byte(bucket))
			if b == nil {
				return errors.New("No bucket " + bucket)
			}
			c := b.Cursor()
			var k, v []byte
			if key == nil {
				k, v = c.First()
			} else {
				log.Println("Seeking")
				k, v = c.Seek(key)
			}

			for ; k != nil; k, v = c.Next() {
				var val interface{}
				err := json.Unmarshal(v, &val)
				if err != nil {
					return err
				}
				ent := DBE{string(k), val}
				select {
				case out <- &ent:
				case <-cancel:
					return nil
				}
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
		close(out)
	}()
	return out
}

type KeyValueInterface interface {
	Set(key string, value interface{})
	Get(key string) interface{}
	Iterator(cancel <-chan struct{}) <-chan Element
}

func (db *DB) Commit(bucket string, item KeyValueInterface) error {
	for ele := range item.Iterator(nil) {
		b, err := json.Marshal(ele.Value())
		if err != nil {
			return err
		}
		db.Put(bucket, []byte(ele.Key()), b)
	}
	return nil
}

func (db *DB) Stats() bolt.Stats {
	bdb := (*bolt.DB)(db)
	return bdb.Stats()
}
