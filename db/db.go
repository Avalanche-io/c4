// The c4 db package wraps the bolt db for use as a simple key value store.
// The keys are typically raw c4 ids, the values are *always* raw c4 ids.
package db

import "github.com/boltdb/bolt"

type DB bolt.DB

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
