package db_test

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"os"
	"testing"

	"github.com/Avalanche-io/c4/db"
	c4 "github.com/Avalanche-io/c4/id"
)

func mkdb(name string, t *testing.T) (*db.DB, func() error, error) {
	dir, err := ioutil.TempDir("", "c4_tests")
	if err != nil {
		return nil, nil, err
	}
	t.Logf("temp folder created at %q", dir)
	tmpdb := filepath.Join(dir, name)
	db, err := db.Open(tmpdb, nil)
	if err != nil {
		return nil, nil, err
	}

	return db, func() error {
		err := db.Close()
		if err != nil {
			return err
		}
		return os.RemoveAll(dir)
	}, nil
}

func TestKeyApi(t *testing.T) {
	db_filename := "test.db"
	db, done, err := mkdb(db_filename, t)
	if err != nil {
		t.Errorf("error opening db at %q: %q", db_filename, err)
	}
	defer done()

	t.Run("Key Set, Get, Find, Delete", func(t *testing.T) {
		id := c4.Identify(strings.NewReader("foo"))
		key := "test/key/path"
		// Set
		old_id, err := db.KeySet(key, id.Digest())
		if err != nil {
			t.Errorf("error setting key: %q", err)
		}
		if old_id != nil {
			t.Errorf("setting an unset key should return a nil digest: %q", err)
		}
		t.Logf("Set %q: %q", key, id)

		// Get
		test_digest, err := db.KeyGet(key)
		if err != nil {
			t.Errorf("error getting key: %q", err)
		}
		if id.Cmp(test_digest.ID()) != 0 {
			t.Errorf("values don't match expected %q, got %q", id, test_digest.ID())
		}
		t.Logf("Get %q: %q", key, test_digest.ID())

		// Find
		for _, found_key := range db.KeyFind(id.Digest()) {
			if found_key != key {
				t.Errorf("keys do not match expecting %q, got %q", key, found_key)
			}
		}

		// Delete
		deleted_digest, err := db.KeyDelete(key)
		if err != nil {
			t.Errorf("unable to delete key %q: %q", key, err)
		}
		if deleted_digest.ID().Cmp(id) != 0 {
			t.Errorf("delete returned incorrect value expected %s, got %s", id, deleted_digest.ID())
		}

	})

	var prefix string
	t.Run("KeyGetAll", func(t *testing.T) {
		rand.Seed(42)

		// Create a map of random keys, and a sorted slice of those keys
		keys := make(map[string]c4.Digest)
		sorted_keys := make([]string, 1000)
		var key string
		alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		prefix = path.Join("test", "prefix")
		for i := range sorted_keys {

			// Every 50th key we choose a random letter to put in the path to make
			// the sorting a little more interesting, and more representative of
			// actual use.
			if i%50 == 0 {
				key = path.Join(prefix, string(alphabet[rand.Int()%len(alphabet)]))
			}

			// Setting key, and value
			k := path.Join(key, strconv.Itoa(rand.Int()))
			v := randomDigest()
			sorted_keys[i] = k
			keys[k] = v
		}
		sort.Strings(sorted_keys)

		// Set the keys in the database.
		for k, v := range keys {
			_, err := db.KeySet(k, v)
			if err != nil {
				t.Errorf("error setting key: %q, %q", k, err)
			}
		}

		// Test KeyGetAll with empty key
		var count int
		for en := range db.KeyGetAll("") {
			if en.Err() != nil {
				t.Errorf("error in KeyGetAll %q", en.Err())
			}
			v := en.Value()
			k := en.Key()

			if keys[k].ID().Cmp(v.ID()) != 0 {
				t.Errorf("values don't match for key %q: %q, %q", k, keys[k].ID(), v.ID())
			}
			en.Close()
			count++
		}
		if count != len(sorted_keys) {
			t.Errorf("wrong number of keys returned %d of %d", count, len(sorted_keys))
		}

		// We pick an arbitrary key, and trim it to create a prefix
		prefix = path.Dir(sorted_keys[115])
		count = 0
		// We expect the keys for the prefix of 115 to progress in
		// order starting at 100 since above we used %50
		i := 100
		for en := range db.KeyGetAll(prefix) {
			if en.Err() != nil {
				t.Errorf("error in KeyGetAll %q", en.Err())
			}
			if sorted_keys[i] != en.Key() {
				t.Errorf("keys not equal %q, %q", sorted_keys[i], en.Key())
			}
			en.Close()
			count++
			i++
		}
		if count != 50 || i != 150 {
			t.Errorf("wrong number of keys returned in prefix search, got %d expected 50", count)
		}

	})

	t.Run("KeyDeleteAll", func(t *testing.T) {

		n, err := db.KeyDeleteAll(prefix)
		if err != nil {
			t.Errorf("unable to delete all entries with %q prefix %s", prefix, err)
		}
		if n != 50 {
			t.Errorf("unable to delete all entries with %q prefix, expected 50, got %d", prefix, n)
		}

		n, err = db.KeyDeleteAll()
		if err != nil {
			t.Errorf("unable to delete all entries, %q", err)
		}
		if n != 950 {
			t.Errorf("unable to delete all entries, expected 950, got %d", n)
		}

		i := 0
		for en := range db.KeyGetAll("") {
			if en.Err() != nil {
				t.Errorf("error in KeyGetAll %q", en.Err())
			}
			i++
			en.Close()
		}
		if i != 0 {
			t.Errorf("not all entries delete %d remain", i)
		}
	})

	t.Run("KeyCAS", func(t *testing.T) {
		id_foo := c4.Identify(strings.NewReader("foo"))
		id_bar := c4.Identify(strings.NewReader("bar"))
		id_bat := c4.Identify(strings.NewReader("bat"))
		key := "test compare and swap"

		// Set initial value
		_, err := db.KeySet(key, id_foo.Digest())
		if err != nil {
			t.Errorf("error setting key: %q", err)
		}

		// Test the positive case
		if !db.KeyCAS(key, id_foo.Digest(), id_bar.Digest()) {
			t.Errorf("compare and swap operation failed on valid compare")
		}

		// Expecting fail since the key should now be set to id_bar, not id_foo
		if db.KeyCAS(key, id_foo.Digest(), id_bat.Digest()) {
			t.Errorf("compare and swap operation succeeded on invalid compare")
		}

		// Test nil handling
		if !db.KeyCAS(key, id_bar.Digest(), nil) {
			t.Errorf("compare and swap operation failed on valid compare")
		}

		// Sets key only if existing value is nil
		if !db.KeyCAS(key, nil, id_bat.Digest()) {
			t.Errorf("compare and swap operation failed on valid compare")
		}

	})

	// db.KeyClock(key) uint64

}

func TestLinkApi(t *testing.T) {
	db_filename := "test.db"
	db, done, err := mkdb(db_filename, t)
	if err != nil {
		t.Errorf("error opening db at %q: %q", db_filename, err)
	}
	defer done()

	_ = db

	t.Run("Link Set, Get, Delete", func(t *testing.T) {
		foo_id := c4.Identify(strings.NewReader("foo"))
		fooAttr_id := c4.Identify(strings.NewReader("attributes of foo"))

		// Set
		err := db.LinkSet("attributes", foo_id.Digest(), fooAttr_id.Digest())
		if err != nil {
			t.Errorf("error setting link: %q", err)
		}

		// Get
		count := 0

		for en := range db.LinkGet("attributes", foo_id.Digest()) {
			if en.Err() != nil {
				t.Errorf("error getting link: %q", en.Err())
				en.Stop()
				en.Close()
				break
			}
			if fooAttr_id.Cmp(en.Target().ID()) != 0 {
				t.Errorf("error getting link")
			}
			en.Close()
			count++
		}
		if count != 1 {
			t.Errorf("incorrect link count %d", count)
		}

		// Delete
		n, err := db.LinkDelete("attributes", foo_id.Digest(), fooAttr_id.Digest())
		if err != nil {
			t.Errorf("error failed to delete link %q", err)
		}
		if n != 1 {
			t.Errorf("failed to delete link")
		}

	})

	var delete_digest c4.Digest
	t.Run("LinkGetAll", func(t *testing.T) {
		rand.Seed(42)
		// Create a slice of "source" digests
		digests := make([]c4.Digest, 1000)
		for i := range digests {
			digests[i] = randomDigest()
		}
		delete_digest = digests[42]

		relationships := []string{"metadata", "parent", "fileinfo"}

		// Link sources to random "target" digests with 1 to 3 per relationship.
		expected_digests := make(map[string][]c4.Digest)
		expected_relationships := make(map[string]int)
		expected_count := 0
		for _, digest := range digests {
			for _, relationship := range relationships {
				targets := make([]c4.Digest, rand.Int()%3+1)
				for k := range targets {
					targets[k] = randomDigest()
					expected_count++
				}
				expected_relationships[digest.ID().String()+relationship] = len(targets)
				expected_digests[digest.ID().String()+relationship] = targets
				err := db.LinkSet(relationship, digest, targets...)
				if err != nil {
					t.Errorf("error setting link: %q", err)
				}
			}
		}
		t.Logf("set %d links", expected_count)
		// get all links and confirm they are correct
		relationship_counts := make(map[string]int)
		count := 0
		log_limit := 19
		for _, digest := range digests {
			if count < log_limit {
				t.Logf("source %s\n", digest.ID())
			}

			for en := range db.LinkGetAll(digest) {
				if en.Err() != nil {
					t.Errorf("error in LinkGetAll %q", en.Err())
				}

				if digest.ID().Cmp(en.Source().ID()) != 0 {
					t.Errorf("LinkGetAll wrong source %q, %q", digest.ID(), en.Source().ID())
				}
				relationship := en.Relationships()[0]

				// 6 == length of "parent"
				if len(relationship) < 6 {
					t.Errorf("invalid relationship %q", relationship)
				}
				relationship_counts[digest.ID().String()+relationship] += 1
				found := false
				for _, target := range expected_digests[digest.ID().String()+relationship] {
					if target.ID().Cmp(en.Target().ID()) == 0 {
						found = true
						break
					}
				}
				en.Close()
				if found {
					count++
				} else {
					t.Errorf("link not found")
				}
				if count < log_limit {
					t.Logf("\t%10q:\t%s\n", relationship, en.Target().ID())
				}
			}
			for _, relationship := range relationships {
				key := digest.ID().String() + relationship
				if relationship_counts[key] == 0 || relationship_counts[key] != expected_relationships[key] {
					t.Errorf("incorrect relationship count for %q expected %d, got %d", relationship, expected_relationships[key], relationship_counts[key])
				}
			}
		}
		t.Logf("got %d links", count)
		if count != expected_count {
			t.Errorf("failed to return all links expected %d, got %d", expected_count, count)
		}

	})

	t.Run("LinkDeleteAll", func(t *testing.T) {
		// db.LinkDeleteAll(id.Digest) (int, error)
		n, err := db.LinkDeleteAll(delete_digest)
		if err != nil {
			t.Errorf("unable to delete all entries %s", err)
		}
		if n != 5 {
			t.Errorf("unable to delete all entries, expected 5, got %d", n)
		}

		n, err = db.LinkDeleteAll()
		if err != nil {
			t.Errorf("unable to delete all entries, %q", err)
		}
		if n != 5939 {
			t.Errorf("unable to delete all entries, expected 5973, got %d", n)
		}
		st := db.Stats()
		t.Logf("Stats Trees:%d, Keys:%d, Indexes: %d, Links:%d, TreesSize:%d(%d)\n", st.Trees, st.Keys, st.KeyIndexes, st.Links, st.TreesSize, st.TreesSize/64)

		i := 0
		for en := range db.LinkGetAll() {
			if en.Err() != nil {
				t.Errorf("error in KeyGetAll %q", en.Err())
			}
			i++
			en.Close()
		}
		if i != 0 {
			t.Errorf("not all entries delete %d remain", i)
		}
	})

}

func TestTreeApi(t *testing.T) {
	db_filename := "test.db"
	db, done, err := mkdb(db_filename, t)
	if err != nil {
		t.Errorf("error opening db at %q: %q", db_filename, err)
	}
	defer done()

	_ = db

	t.Run("Tree Set, Get, Delete", func(t *testing.T) {
		// Create a tree
		var digests c4.DigestSlice
		for i := 0; i < 100; i++ {
			digests.Insert(randomDigest())
		}
		tree := c4.NewTree(digests)
		tree_digest := tree.Compute()

		err := db.TreeSet(tree)
		if err != nil {
			t.Errorf("error setting tree %q", err)
		}

		tree2, err := db.TreeGet(tree_digest)
		if err != nil {
			t.Errorf("error getting tree %q", err)
		}

		if tree2 == nil || tree2.ID().Cmp(tree_digest.ID()) != 0 {
			id_str := "<nil>"
			if tree2 != nil {
				id_str = tree2.ID().String()
			}
			t.Errorf("error tree ids don't match expected %q, got %q", tree_digest.ID(), id_str)
		}

		st := db.Stats()
		if st.Trees != 1 || st.TreesSize/64 != 202 {
			t.Errorf("error tree has incorrect stats before delete")
		}
		t.Logf("Stats Trees:%d, Keys:%d, Indexes: %d, Links:%d, TreesSize:%d(%d)\n", st.Trees, st.Keys, st.KeyIndexes, st.Links, st.TreesSize, st.TreesSize/64)
		err = db.TreeDelete(tree_digest)
		if err != nil {
			t.Errorf("failed to delete tree")
		}
		st = db.Stats()
		if st.Trees != 0 || st.TreesSize != 0 {
			t.Errorf("error tree has incorrect stats after delete")
			t.Errorf("Stats Trees:%d, Keys:%d, Indexes: %d, Links:%d, TreesSize:%d(%d)\n", st.Trees, st.Keys, st.KeyIndexes, st.Links, st.TreesSize, st.TreesSize/64)
		}
	})

}

func TestBatching(t *testing.T) {
	db_filename := "test.db"
	c4db, done, err := mkdb(db_filename, t)
	if err != nil {
		t.Errorf("error opening db at %q: %q", db_filename, err)
	}
	defer done()

	c4db.KeyBatch(func(tx *db.Tx) bool {
		for i := 0; i < 100000; i++ {
			tx.KeySet(strconv.Itoa(i), randomDigest())
			if tx.Err() != nil {
				t.Errorf("error during batch write")
				return false
			}
		}
		return false
	})

	st := c4db.Stats()
	t.Logf("Stats Trees:%d, Keys:%d, Indexes: %d, Links:%d, TreesSize:%d(%d)\n", st.Trees, st.Keys, st.KeyIndexes, st.Links, st.TreesSize, st.TreesSize/64)
	if st.Keys != 100000 {
		t.Errorf("error tree has incorrect stats after delete")
		t.Errorf("Stats Trees:%d, Keys:%d, Indexes: %d, Links:%d, TreesSize:%d(%d)\n", st.Trees, st.Keys, st.KeyIndexes, st.Links, st.TreesSize, st.TreesSize/64)
	}

}

// utility to create a random c4.Digest
func randomDigest() c4.Digest {
	// Create some random bytes.
	var data [8]byte
	rand.Read(data[:])
	return c4.Identify(bytes.NewReader(data[:])).Digest()
}
