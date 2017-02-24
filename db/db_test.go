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

func TestDBOpenClose(t *testing.T) {
	_, _, Teardown := Setup(t)
	defer Teardown()
}

func TestDBSetGet(t *testing.T) {
	is, db, Teardown := Setup(t)
	defer Teardown()

	in_id, err := c4id.Identify(bytes.NewReader([]byte("bar")))
	is.NoErr(err)
	is.NotNil(in_id)
	err = db.SetAssetID([]byte("foo"), in_id)
	is.NoErr(err)
	out_id := db.GetAssetID([]byte("foo"))
	is.NoErr(err)
	is.NotNil(out_id)

	is.Equal(in_id.String(), out_id.String())

	err = db.SetAttributesID([]byte("foo"), in_id)
	is.NoErr(err)
	out_id = db.GetAttributesID([]byte("foo"))
	is.NoErr(err)
	is.NotNil(out_id)

	is.Equal(in_id.String(), out_id.String())
}

func TestDBForEach(t *testing.T) {
	is, db, Teardown := Setup(t)
	defer Teardown()

	m := make(map[string][][]byte)

	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("%d", i))
		asset_value := []byte(fmt.Sprintf("%d", rand.Int()))
		attribute_value := []byte(fmt.Sprintf("%d", rand.Int()))
		m[string(key)] = [][]byte{asset_value, attribute_value}
		id, err := c4id.Identify(bytes.NewReader(asset_value))
		is.NoErr(err)
		is.NotNil(id)
		err = db.SetAssetID(key, id)

		id, err = c4id.Identify(bytes.NewReader(attribute_value))
		is.NoErr(err)
		is.NotNil(id)
		is.NoErr(err)
		err = db.SetAttributesID(key, id)
		is.NoErr(err)
	}

	db.ForEach(func(key []byte, asset_id *c4id.ID, attribute_id *c4id.ID) error {
		expected := m[string(key)]
		values := []*c4id.ID{asset_id, attribute_id}
		for i, v := range values {
			id, err := c4id.Identify(bytes.NewReader(expected[i]))
			is.NoErr(err)
			is.NotNil(id)
			is.Equal(id, v)
		}
		return nil
	})
}
