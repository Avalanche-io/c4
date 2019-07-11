package db

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	c4 "github.com/Avalanche-io/c4/id"
	"github.com/boltdb/bolt"
)

// updated, delete me

// DB stores
type DB struct {
	// Bolt database interface
	db *bolt.DB

	// List of paths in which external files might be found
	storage []string

	// tree storage settings
	treeMaxSize  int
	treeStrategy TreeStrategyType
}

// func init() {
// 	bucketList := [][]byte{keyBucket, linkBucket}
// }
type TreeStrategyType int

const (
	TreeStrategyNone TreeStrategyType = iota

	// Sets the tree storage strategy to always store the entire tree.
	TreeStrategyCache

	// Sets the tree storage strategy to always store only the id list, and
	// compute the tree when restored.
	TreeStrategyCompute

	// Sets the tree storage strategy to automatically balance between the
	// cashing strategy and computing strategy.
	TreeStrategyBalance
)

type Options struct {
	// Maximum size in bytes of a tree stored directly in the database.  Trees
	// over this size will be written out to separate files.
	TreeMaxSize int

	// Sets the strategy to use for tree storage.
	TreeStrategy TreeStrategyType

	// Path to an alternative storage location.
	ExternalStore []string
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

type Tx struct {
	db       *bolt.DB
	count    int
	chanchan chan chan *entry
	enCh     chan *entry
	errCh    chan error
}

func (t *Tx) KeySet(key string, digest c4.Digest) {
	en := entry_pool.Get().(*entry)
	en.k = []byte(key)
	en.v = digest
	t.enCh <- en
	t.count++
	if t.count%10000 == 0 {
		close(t.enCh)
		t.enCh = make(chan *entry)
		t.chanchan <- t.enCh
	}
}

func (t *Tx) Err() error {
	select {
	case err := <-t.errCh:
		return err
	default:
	}
	return nil
}

func (t *Tx) close() {
	close(t.enCh)
	close(t.chanchan)
	close(t.errCh)
}

var (
	// root bucket
	c4Bucket []byte = []byte("c4")
	// key -> id bucket
	keyBucket []byte = []byte("key")
	// id -> key bucket
	indexBucket []byte = []byte("index")
	// id -> id bucket
	linkBucket []byte = []byte("link")
	treeBucket []byte = []byte("tree")
	pathBucket []byte = []byte("path")

	statsBucket   []byte = []byte("stats")
	optionsBucket []byte = []byte("options")
)

var bucketList [][]byte = [][]byte{
	keyBucket,
	indexBucket,
	linkBucket,
	treeBucket,
	statsBucket,
	optionsBucket,
	pathBucket,
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Open opens or initializes a DB for the given path. When creating
// file permissions are set to 0700, when opening an existing database file
// permissions are not modified.
func Open(path string, options *Options) (db *DB, err error) {

	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.Mkdir(path, 0700)
		if err != nil {
			return nil, err
		}
	}

	db_path := filepath.Join(path, "db")
	db = new(DB)
	db.db, err = bolt.Open(db_path, 0700, nil)
	if err != nil {
		return nil, err
	}

	err = db.db.Update(func(t *bolt.Tx) error {
		root, err := t.CreateBucketIfNotExists(c4Bucket)
		if err != nil {
			return err
		}

		for _, bucket := range bucketList {
			_, err := root.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	saved_options := db.read_options()
	if options == nil {
		options = saved_options
		if options == nil {
			options = new(Options)
		}
	} else {
		if options.ExternalStore != nil || len(options.ExternalStore) >= 0 {
			db.storage = append(db.storage, options.ExternalStore...)
		}
		db.treeMaxSize = saved_options.TreeMaxSize
		if options.TreeMaxSize > 0 {
			db.treeMaxSize = options.TreeMaxSize
		}
		db.treeStrategy = saved_options.TreeStrategy
		if options.TreeStrategy != TreeStrategyNone {
			db.treeStrategy = options.TreeStrategy
		}
	}
	db.write_options()

	if len(db.storage) == 0 {
		db.storage = append(db.storage, path)
	}
	return db, nil
}

func (db *DB) write_options() {
	data, err := json.Marshal(Options{
		ExternalStore: db.storage,
		TreeStrategy:  db.treeStrategy,
		TreeMaxSize:   db.treeMaxSize,
	})
	if err != nil {
		return
	}

	// TODO: might be better just to save this as a YAML file
	db.db.Update(func(t *bolt.Tx) error {
		b := t.Bucket(c4Bucket).Bucket(optionsBucket)
		return b.Put([]byte("global/options"), data)
	})
}

func (db *DB) read_options() *Options {
	var opts *Options
	db.db.View(func(t *bolt.Tx) error {
		b := t.Bucket(c4Bucket).Bucket(optionsBucket)

		data := b.Get([]byte("global/options"))
		if data == nil {
			return nil
		}
		opts = new(Options)
		json.Unmarshal(data, opts)
		return nil
	})
	return opts
}

func (db *DB) Close() error {
	return db.db.Close()
}

type Stats struct {
	Keys       int
	KeyIndexes int
	Trees      int
	Links      int
	TreesSize  uint64
}

func (db *DB) Stats() *Stats {
	var st Stats
	err := db.db.View(func(t *bolt.Tx) error {
		info := t.Bucket(c4Bucket).Bucket(keyBucket).Stats()
		st.Keys = info.KeyN
		info = t.Bucket(c4Bucket).Bucket(indexBucket).Stats()
		st.KeyIndexes = info.KeyN
		info = t.Bucket(c4Bucket).Bucket(linkBucket).Stats()
		st.Links = info.KeyN
		info = t.Bucket(c4Bucket).Bucket(treeBucket).Stats()
		st.Trees = info.KeyN
		if info.LeafInuse >= 96 {
			st.TreesSize = uint64(info.LeafInuse - 96)
		}
		// keyb := t.Bucket(keyBucket)
		// c := keyb.Cursor()

		// for k, _ := c.First(); k != nil; k, _ = c.Next() {
		// 	st.Keys++
		// }
		// linkb := t.Bucket(linkBucket)
		// c = linkb.Cursor()
		// for k, _ := c.First(); k != nil; k, _ = c.Next() {
		// 	st.Links++
		// }
		// treeb := t.Bucket(treeBucket)
		// c = treeb.Cursor()
		// for k, v := c.First(); k != nil; k, v = c.Next() {
		// 	st.Trees++
		// 	st.TreesSize += uint64(len(v))
		// }
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
		b := t.Bucket(c4Bucket).Bucket(keyBucket)
		data := b.Get(k)
		err := b.Put(k, digest)
		if err != nil {
			return err
		}

		xb := t.Bucket(c4Bucket).Bucket(indexBucket)
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
	db.db.View(func(t *bolt.Tx) error {
		// Assume bucket exists and has keys
		c := t.Bucket(c4Bucket).Bucket(indexBucket).Cursor()

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
		data := t.Bucket(c4Bucket).Bucket(keyBucket).Get([]byte(key))
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
		b := t.Bucket(c4Bucket).Bucket(keyBucket)
		data := b.Get([]byte(key))
		b.Delete([]byte(key))
		if data == nil {
			return nil
		}
		value = make([]byte, 64)
		copy(value, data)

		bx := t.Bucket(c4Bucket).Bucket(indexBucket)
		kx := append(data, []byte(key)...)
		return bx.Delete(kx)
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}

type entry struct {
	// key, source
	k []byte

	// value, target
	v []byte

	// err
	e error

	// relationships
	r []byte

	// stop
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

		db.db.View(func(t *bolt.Tx) error {
			// Assume bucket exists and has keys
			c := t.Bucket(c4Bucket).Bucket(keyBucket).Cursor()

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
		b := t.Bucket(c4Bucket).Bucket(keyBucket)
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
			b := t.Bucket(c4Bucket).Bucket(keyBucket)
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
		err := db.db.Update(func(t *bolt.Tx) error {
			// Assume bucket exists and has keys
			b := t.Bucket(c4Bucket).Bucket(keyBucket)
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
		b := t.Bucket(c4Bucket).Bucket(linkBucket)

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
			c := t.Bucket(c4Bucket).Bucket(linkBucket).Cursor()

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
			b := t.Bucket(c4Bucket).Bucket(linkBucket)
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
				c := t.Bucket(c4Bucket).Bucket(linkBucket).Cursor()

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
				c := t.Bucket(c4Bucket).Bucket(linkBucket).Cursor()

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
			b := t.Bucket(c4Bucket).Bucket(linkBucket)
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
			b := t.Bucket(c4Bucket).Bucket(linkBucket)
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
	if db.treeMaxSize == 0 || len(data) <= db.treeMaxSize {
		return db.db.Update(func(t *bolt.Tx) error {
			b := t.Bucket(c4Bucket).Bucket(treeBucket)
			return b.Put(data[:64], data)
		})
	}
	path, err := write_file_data(db.storage, data[:64], data)
	if err != nil {
		return err
	}
	// TODO: store the path to the file
	_ = path

	return db.db.Update(func(t *bolt.Tx) error {
		pb := t.Bucket(c4Bucket).Bucket(pathBucket)
		err := pb.Put(data[:64], []byte(path))
		if err != nil {
			return err
		}
		b := t.Bucket(c4Bucket).Bucket(treeBucket)
		return b.Put(data[:64], data[:64])
	})
}

func (db *DB) TreeGet(tree_digest c4.Digest) (*c4.Tree, error) {
	var tree *c4.Tree
	var path string
	err := db.db.View(func(t *bolt.Tx) error {
		b := t.Bucket(c4Bucket).Bucket(treeBucket)
		data := b.Get(tree_digest)
		if data == nil {
			return nil
		}
		if len(data) == 64 {
			pb := t.Bucket(c4Bucket).Bucket(treeBucket)
			path_data := pb.Get(data)
			if path_data != nil {
				path = string(path_data)
			}
			return nil
		}
		tree = new(c4.Tree)
		tree.UnmarshalBinary(data)
		return nil
	})
	if len(path) > 0 {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		tree = new(c4.Tree)
		tree.UnmarshalBinary(data)
	}
	return tree, err
}

func (db *DB) TreeDelete(tree c4.Digest) error {
	return db.db.Update(func(t *bolt.Tx) error {
		b := t.Bucket(c4Bucket).Bucket(treeBucket)
		return b.Delete(tree)
	})
}

func (db *DB) KeyBatch(f func(*Tx) bool) {

	// To write to the db in batches of 10k items, we create a channel
	// of channels, starting a new batch after each inner channel is closed.
	t := new(Tx)
	t.db = db.db
	t.chanchan = make(chan chan *entry)
	t.enCh = make(chan *entry)
	t.errCh = make(chan error, 1)

	go func() {
		for dbin := range t.chanchan {
			t.db.Batch(func(tx *bolt.Tx) error {
				b := tx.Bucket(c4Bucket).Bucket(keyBucket)
				xb := tx.Bucket(c4Bucket).Bucket(indexBucket)
				for en := range dbin {
					data := b.Get(en.k)
					err := b.Put(en.k, en.v)
					if err != nil {
						return err
					}
					xk := append(en.v, en.k...)
					err = xb.Put(xk, []byte{byte(1)})
					if err != nil {
						return err
					}
					// If there was a value set on the key previously we must copy the bytes.
					if data != nil {
						// previous = make([]byte, 64)
						// copy(previous, data)
						data = append(data, en.k...)
						err := xb.Delete(xk)
						if err != nil {
							return err
						}
					}
					entry_pool.Put(en)

					if err != nil {
						select {
						case t.errCh <- err:
						default:
						}
					}
				}
				return nil
			})
		}
	}()

	t.chanchan <- t.enCh

	for f(t) {
	}

	t.close()

	return
}

// write_file_data first cycles through the paths in db.storage to check
// if the file already exists.  Then, if not it goes back to the first
// path in the list and attempts to write the file.  If that fails
// it will continue trying paths until one succeeds or they all fail.
func write_file_data(paths []string, digest c4.Digest, data []byte) (string, error) {

	filename := digest.ID().String()
	var save_paths []string

	// range over storage and check for existence.
	for _, path := range paths {
		// Instead of attempting to create folders that may be above the
		// actual storage location we return an error.
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return "", err
		}

		// Sub folders to partition the C4 name space a little.
		dir := filepath.Join(path, filename[0:2], filename[2:4])
		fullfilepath := filepath.Join(dir, filename)

		// If the file already exists we're done.
		if info, err := os.Stat(fullfilepath); err == nil {
			// A very basic sanity check just in case a previous write didn't finish
			if info.Size() == int64(len(data)) {
				return fullfilepath, nil
			}
		}

		save_paths = append(save_paths, fullfilepath)
	}
	// shuffle the possible storage locations so that we distribute new
	// files evenly.
	shuffle(save_paths)

	// range over possible storage locations trying to save in each one.
	for _, path := range save_paths {
		dir := filepath.Dir(path)
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			return "", err
		}
		f, err := os.Create(filename)
		if err != nil {
			continue
		}
		f.Write(data)
		f.Close()
		return path, nil
	}

	// Should never reach here.
	return "", nil
}

// Update, View and Batch call the methods of the same name on the underlying
// bolt database. See github.com/boltdb/bolt for more information.
//
// Update executes a function within the context of a read-write managed transaction.
// If no error is returned from the function then the transaction is committed.
// If an error is returned then the entire transaction is rolled back.
// Any error that is returned from the function or returned from the commit is
// returned from the Update() method.
//
// Attempting to manually commit or rollback within the function will cause a panic.
func (db *DB) Update(fn func(*bolt.Tx) error) error {
	return db.db.Update(fn)
}

// Update, View and Batch call the methods of the same name on the underlying
// bolt database. See github.com/boltdb/bolt for more information.
//
// View executes a function within the context of a managed read-only transaction.
// Any error that is returned from the function is returned from the View() method.
//
// Attempting to manually rollback within the function will cause a panic.
func (db *DB) View(fn func(*bolt.Tx) error) error {
	return db.db.View(fn)
}

// Update, View and Batch call the methods of the same name on the underlying
// bolt database. See github.com/boltdb/bolt for more information.
//
// Batch calls fn as part of a batch. It behaves similar to Update,
// except:
//
// 1. concurrent Batch calls can be combined into a single Bolt
// transaction.
//
// 2. the function passed to Batch may be called multiple times,
// regardless of whether it returns error or not.
//
// This means that Batch function side effects must be idempotent and
// take permanent effect only after a successful return is seen in
// caller.
//
// The maximum batch size and delay can be adjusted with DB.MaxBatchSize
// and DB.MaxBatchDelay, respectively.
//
// Batch is only useful when there are multiple goroutines calling it.
func (db *DB) Batch(fn func(*bolt.Tx) error) error {
	return db.db.Batch(fn)
}

func shuffle(list []string) {
	l := len(list)
	for j, i := 0, 0; i < l; i++ {
		j = int(rand.Int31n(int32(l)))
		if j != i {
			list[i], list[j] = list[j], list[i]
		}
	}
	return
}
