package db

import (
	"bytes"
	"errors"
	"sync"
	// "os"

	c4 "github.com/Avalanche-io/c4/id"
	"github.com/Avalanche-io/counter"
	"github.com/boltdb/bolt"
)

type DB struct {
	// Bolt database interface
	db *bolt.DB

	// Global atomic counter
	c counter.Counter
}

var (
	keyBucket   []byte = []byte("key")
	indexBucket []byte = []byte("index")
	linkBucket  []byte = []byte("link")
	treeBucket  []byte = []byte("tree")
	statsBucket []byte = []byte("stats")
)

var bucketList [][]byte = [][]byte{keyBucket, indexBucket, linkBucket, treeBucket, statsBucket}

// func init() {
// 	bucketList := [][]byte{keyBucket, linkBucket}
// }

type Options struct {
	// Nothing here yet, but we keep it as an API place holder.

}

// Entry is an interface for items returned from listing methods like
// KeyGetAll. For performance an Entry must be closed when done using it.
type Entry interface {
	Key() string
	Value() c4.Digest
	Source() c4.Digest
	Target() c4.Digest
	Relationships() []string
	Err() error
	Stop()
	Close()
}

// Open opens or initializes a DB for the given path. When creating
// file permissions are set to 0700, when opening an existing database file
// permissions are not modified.
func Open(path string, options *Options) (db *DB, err error) {
	db = new(DB)

	db.db, err = bolt.Open(path, 0700, nil)
	if err != nil {
		return nil, err
	}

	err = db.db.Update(func(t *bolt.Tx) error {
		for _, bucket := range bucketList {
			_, err := t.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return db, err
}

func (db *DB) Close() error {
	return db.db.Close()
}

type Stats struct {
	Trees     int
	Keys      int
	Links     int
	TreesSize uint64
}

func (db *DB) Stats() *Stats {
	var st Stats
	err := db.db.View(func(t *bolt.Tx) error {
		keyb := t.Bucket(keyBucket)
		c := keyb.Cursor()

		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			st.Keys++
		}
		linkb := t.Bucket(linkBucket)
		c = linkb.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			st.Links++
		}
		treeb := t.Bucket(treeBucket)
		c = treeb.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			st.Trees++
			st.TreesSize += uint64(len(v))
		}
		return nil
	})
	if err != nil {
		return nil
	}

	return &st
}

// KeySet stores a digest with the key provided.  If the key was previously
// set the previous digest is returned otherwise nil is returned.
func (db *DB) KeySet(key string, digest c4.Digest) (c4.Digest, error) {
	var previous []byte
	err := db.db.Update(func(t *bolt.Tx) error {
		k := []byte(key)
		b := t.Bucket(keyBucket)
		data := b.Get(k)
		err := b.Put(k, digest)
		if err != nil {
			return err
		}

		xb := t.Bucket(indexBucket)
		xk := append(digest, k...)
		err = xb.Put(xk, []byte{byte(1)})
		if err != nil {
			return err
		}
		// If there was a value set on the key previously we must copy the bytes.
		if data != nil {
			previous = make([]byte, 64)
			copy(previous, data)
			xk = append(previous, k...)
			err := xb.Delete(xk)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return previous, nil
}

func (db *DB) KeyFind(digest c4.Digest) []string {
	var keys []string
	db.db.View(func(tx *bolt.Tx) error {
		// Assume bucket exists and has keys
		c := tx.Bucket(indexBucket).Cursor()

		for k, _ := c.Seek(digest); k != nil && bytes.HasPrefix(k, digest); k, _ = c.Next() {
			keys = append(keys, string(k[64:]))
		}
		return nil
	})
	return keys
}

func (db *DB) KeyGet(key string) (c4.Digest, error) {
	var value []byte
	err := db.db.View(func(t *bolt.Tx) error {
		data := t.Bucket(keyBucket).Get([]byte(key))
		if data != nil {
			value = make([]byte, 64)
			copy(value, data)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (db *DB) KeyDelete(key string) (c4.Digest, error) {
	var value []byte
	err := db.db.Update(func(t *bolt.Tx) error {
		b := t.Bucket(keyBucket)
		data := b.Get([]byte(key))
		b.Delete([]byte(key))
		if data == nil {
			return nil
		}
		value = make([]byte, 64)
		copy(value, data)

		bx := t.Bucket(indexBucket)
		kx := append(data, []byte(key)...)
		return bx.Delete(kx)
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}

type entry struct {
	k  []byte
	v  []byte
	e  error
	r  []byte
	st chan struct{}
}

func (e *entry) Key() string {
	// Key is called while db transaction is open.  We must copy data before
	// handing it to a process that may use it outside of the transaction.
	data := make([]byte, len(e.k))
	copy(data, e.k)
	return string(data)
}

func (e *entry) Value() c4.Digest {
	data := make([]byte, len(e.v))
	copy(data, e.v)
	return c4.Digest(data)
}

func (e *entry) Close() {
	entry_pool.Put(e)
}

func (e *entry) Source() c4.Digest {
	return c4.Digest(e.k)
}

func (e *entry) Target() c4.Digest {
	return c4.Digest(e.v)
}

// TODO: there may be more than one relationship for a given link
func (e *entry) Relationships() []string {
	return []string{string(e.r)}
}

func (e *entry) Err() error {
	return e.e
}

func (e *entry) Stop() {
	close(e.st)
}

var entry_pool = sync.Pool{
	New: func() interface{} {
		return new(entry)
	},
}

func (db *DB) KeyGetAll(key_prefix ...string) <-chan Entry {
	out := make(chan Entry)
	stop := make(chan struct{})
	go func() {
		defer func() {
			close(out)
			close(stop)
		}()

		db.db.View(func(tx *bolt.Tx) error {
			// Assume bucket exists and has keys
			c := tx.Bucket(keyBucket).Cursor()

			for _, key_prefix := range key_prefix {
				prefix := []byte(key_prefix)
				for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {

					ent := entry_pool.Get().(*entry)
					ent.k = k
					ent.v = v
					if len(v) != 64 {
						ent.e = errors.New("wrong value size")
					}
					ent.st = stop

					select {
					case out <- ent:
					case <-stop:
						return nil
					}

				}
			}
			return nil
		})
	}()
	return out
}

// KeyCAS implements a 'compare and swap' operation on key. In other words
// if the value of key is not the expected `old_digest` value the operation will
// do noting and return false. If `old_digest` is the current value of key
// the key is updated with new_digest, KeyCAS returns `true`.
func (db *DB) KeyCAS(key string, old_digest, new_digest c4.Digest) bool {
	var replaced bool
	db.db.Update(func(t *bolt.Tx) error {
		b := t.Bucket(keyBucket)
		data := b.Get([]byte(key))

		if bytes.Compare(data, old_digest) != 0 {
			return nil
		}

		err := b.Put([]byte(key), []byte(new_digest))
		if err != nil {
			return err
		}
		replaced = true
		return nil
	})
	return replaced
}

// KeyDeleteAll deletes all keys with the provided prefix, or all keys
// in the case of an empty string. KeyDeleteAll returns the number of items
// deleted and/or an error.
func (db *DB) KeyDeleteAll(key_prefixs ...string) (int, error) {
	count := 0
	if len(key_prefixs) == 0 {
		db.db.Update(func(t *bolt.Tx) error {
			b := t.Bucket(keyBucket)
			c := b.Cursor()

			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				err := b.Delete(k)
				if err != nil {
					return err
				}
				count++
			}
			return nil
		})

	}
	for _, key_prefix := range key_prefixs {
		err := db.db.Update(func(tx *bolt.Tx) error {
			// Assume bucket exists and has keys
			b := tx.Bucket(keyBucket)
			c := b.Cursor()

			prefix := []byte(key_prefix)
			for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
				err := b.Delete(k)
				if err != nil {
					return err
				}
				count++
			}

			return nil
		})
		if err != nil {
			return count, err
		}

	}

	return count, nil
}

// LinkSet creates a relationship between a `source` digest and one or more
// `target` digests.  The relationship is an arbitrary string that is
// application dependent, but could be something like "metadata", "parent",
// or "children" for example.
func (db *DB) LinkSet(relationship string, source c4.Digest, targets ...c4.Digest) error {
	// A link key contains both digests in the relationship, 'source' + 'target'
	if len(targets) < 1 {
		return errors.New("missing targets")
	}
	key := make([]byte, 128)
	copy(key, source)

	return db.db.Update(func(t *bolt.Tx) error {
		b := t.Bucket(linkBucket)

		for i := range targets {
			copy(key[64:], targets[i])
			err := b.Put([]byte(key), []byte(relationship))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) LinkGet(relationship string, source c4.Digest) <-chan Entry {
	// A link key contains both digests in the relationship, 'source' + 'target'
	out := make(chan Entry)
	stop := make(chan struct{})
	go func() {
		defer func() {
			close(out)
			close(stop)
		}()
		db.db.View(func(t *bolt.Tx) error {
			c := t.Bucket(linkBucket).Cursor()

			for k, v := c.Seek(source); k != nil && bytes.HasPrefix(k, source); k, v = c.Next() {
				if string(v) != relationship {
					continue
				}
				ent := entry_pool.Get().(*entry)
				ent.k = source
				ent.v = k[64:]
				ent.r = v // relationship

				select {
				case out <- ent:
				case <-stop:
					return nil
				}

			}
			return nil
		})
	}()
	return out
}

func (db *DB) LinkDelete(relationship string, source c4.Digest, targets ...c4.Digest) (int, error) {
	// A link key contains both digests in the relationship, 'source' + 'target'
	n := 0
	if len(targets) < 1 {
		return n, errors.New("missing targets")
	}
	key := make([]byte, 128)
	copy(key, source)
	for i := range targets {
		copy(key[64:], targets[i])
		err := db.db.Update(func(t *bolt.Tx) error {
			b := t.Bucket(linkBucket)
			c := b.Cursor()
			for k, v := c.Seek(key); k != nil && bytes.HasPrefix(k, source); k, v = c.Next() {
				if string(v) == relationship {
					err := b.Delete(k)
					if err != nil {
						return err
					}
					n++
				}
			}
			return nil
		})
		if err != nil {
			return n, err
		}

	}
	return n, nil

}

func (db *DB) LinkGetAll(sources ...c4.Digest) <-chan Entry {
	// A link key contains both digests in the relationship, 'source' + 'target'
	out := make(chan Entry)
	stop := make(chan struct{})
	go func() {
		defer func() {
			close(out)
			close(stop)
		}()
		if len(sources) == 0 {
			db.db.View(func(t *bolt.Tx) error {
				c := t.Bucket(linkBucket).Cursor()

				for k, v := c.First(); k != nil; k, v = c.Next() {
					ent := entry_pool.Get().(*entry)
					ent.k = k[:64]
					ent.v = k[64:]
					ent.r = v // relationship

					select {
					case out <- ent:
					case <-stop:
						return nil
					}

				}
				return nil
			})
		}
		for _, source := range sources {
			db.db.View(func(t *bolt.Tx) error {
				c := t.Bucket(linkBucket).Cursor()

				for k, v := c.Seek(source); k != nil && bytes.HasPrefix(k, source); k, v = c.Next() {
					ent := entry_pool.Get().(*entry)
					ent.k = source
					ent.v = k[64:]
					ent.r = v // relationship

					select {
					case out <- ent:
					case <-stop:
						return nil
					}

				}
				return nil
			})
		}

	}()
	return out
}

func (db *DB) LinkDeleteAll(sources ...c4.Digest) (int, error) {
	count := 0
	if len(sources) == 0 {
		db.db.Update(func(t *bolt.Tx) error {
			b := t.Bucket(linkBucket)
			c := b.Cursor()

			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				err := b.Delete(k)
				if err != nil {
					return err
				}
				count++
			}
			return nil
		})

	}
	for _, source := range sources {
		err := db.db.Update(func(t *bolt.Tx) error {
			// Assume bucket exists and has keys
			b := t.Bucket(linkBucket)
			c := b.Cursor()

			for k, _ := c.Seek(source); k != nil && bytes.HasPrefix(k, source); k, _ = c.Next() {
				err := b.Delete(k)
				if err != nil {
					return err
				}
				count++
			}

			return nil
		})
		if err != nil {
			return count, err
		}

	}

	return count, nil
}

func (db *DB) TreeSet(tree *c4.Tree) error {
	data, err := tree.MarshalBinary()
	if err != nil {
		return err
	}
	return db.db.Update(func(t *bolt.Tx) error {
		b := t.Bucket(treeBucket)
		return b.Put(data[:64], data)
	})
}

func (db *DB) TreeGet(tree_digest c4.Digest) (*c4.Tree, error) {
	var tree *c4.Tree
	err := db.db.View(func(t *bolt.Tx) error {
		b := t.Bucket(treeBucket)
		data := b.Get(tree_digest)
		if data == nil {
			return nil
		}
		tree = new(c4.Tree)
		tree.UnmarshalBinary(data)
		return nil
	})
	return tree, err
}

func (db *DB) TreeDelete(tree c4.Digest) error {
	return db.db.Update(func(t *bolt.Tx) error {
		b := t.Bucket(treeBucket)
		return b.Delete(tree)
	})
}
