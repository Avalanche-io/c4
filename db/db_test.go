package db_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"path/filepath"

	"os"
	"testing"

	c4db "github.com/Avalanche-io/c4/db"
	c4id "github.com/Avalanche-io/c4/id"
	"github.com/cheekybits/is"
)

func Setup(t *testing.T) (is.I, *c4db.DB, func()) {
	is := is.New(t)
	dir, err := ioutil.TempDir("", "c4_tests")
	is.NoErr(err)

	tmpdb := filepath.Join(dir, "c4.db")
	db, err := c4db.Open(tmpdb, 0700, nil)
	is.NoErr(err)
	is.NotNil(db)

	return is, db, func() {
		err := db.Close()
		is.NoErr(err)
		os.RemoveAll(dir)
	}
}

func TestDBSetGet(t *testing.T) {
	is, db, done := Setup(t)
	defer done()

	in_id := c4id.Identify(bytes.NewReader([]byte("bar")))
	is.NotNil(in_id)
	err := db.Set([]byte("foo"), in_id)
	is.NoErr(err)
	out_id := db.Get([]byte("foo"))
	is.NoErr(err)
	is.NotNil(out_id)

	is.Equal(in_id.String(), out_id.String())

	err = db.SetAttributes([]byte("foo"), in_id)
	is.NoErr(err)
	out_id = db.GetAttributes([]byte("foo"))
	is.NoErr(err)
	is.NotNil(out_id)

	is.Equal(in_id.String(), out_id.String())
}

func TestDBForEach(t *testing.T) {
	is, db, done := Setup(t)
	defer done()

	m := make(map[string][][]byte)

	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("%d", i))
		asset_value := []byte(fmt.Sprintf("%d", rand.Int()))
		attribute_value := []byte(fmt.Sprintf("%d", rand.Int()))
		m[string(key)] = [][]byte{asset_value, attribute_value}
		id := c4id.Identify(bytes.NewReader(asset_value))
		is.NotNil(id)
		err := db.Set(key, id)
		is.NoErr(err)
		id = c4id.Identify(bytes.NewReader(attribute_value))
		is.NotNil(id)
		err = db.SetAttributes(key, id)
		is.NoErr(err)
	}

	db.ForEach(func(key []byte, asset_id *c4id.ID, attribute_id *c4id.ID) error {
		expected := m[string(key)]
		values := []*c4id.ID{asset_id, attribute_id}
		for i, v := range values {
			id := c4id.Identify(bytes.NewReader(expected[i]))
			is.NotNil(id)
			is.Equal(id, v)
		}
		return nil
	})
}

func TestUnset(t *testing.T) {
	is, db, done := Setup(t)
	defer done()

	in_id := c4id.Identify(bytes.NewReader([]byte("bar")))
	is.NotNil(in_id)
	err := db.Set([]byte("foo"), in_id)
	is.NoErr(err)
	is.True(db.IDexists(in_id))
	db.Unset([]byte("foo"))
	is.False(db.IDexists(in_id))
}

func TestIDexists(t *testing.T) {
	is, db, done := Setup(t)
	defer done()

	in_id := c4id.Identify(bytes.NewReader([]byte("bar")))
	is.NotNil(in_id)
	err := db.Set([]byte("foo"), in_id)
	is.NoErr(err)
	is.True(db.IDexists(in_id))
}

func TestRefCounting(t *testing.T) {
	is, db, done := Setup(t)
	defer done()

	in_id := c4id.Identify(bytes.NewReader([]byte("bar")))
	is.NotNil(in_id)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("foo%04d", i)
		db.Set([]byte(key), in_id)
	}
	for i := 0; i < 999; i++ {
		key := fmt.Sprintf("foo%04d", i)
		id := db.Unset([]byte(key))
		is.True(id.String() == in_id.String())
		is.True(db.IDexists(in_id))
	}
	id := db.Unset([]byte("foo0999"))
	is.True(id.String() == in_id.String())
	is.False(db.IDexists(in_id))
}
