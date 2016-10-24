package db_test

import (
	"fmt"
	"testing"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/db"
	"github.com/etcenter/c4/test"
)

func TestCreateKVStore(t *testing.T) {
	is := is.New(t)
	item := db.NewKV()
	is.NotNil(item)
}

func TestCreateKVStoreGetSet(t *testing.T) {
	is := is.New(t)
	item := db.NewKV()
	is.NotNil(item)

	item.Set("foo", "bar")
	v := item.Get("foo")
	is.Equal(v.(string), "bar")
}

func TestCreateKVStoreIterator(t *testing.T) {
	is := is.New(t)
	item := db.NewKV()
	is.NotNil(item)

	key_list := map[string]int{}
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("%08d", i)
		key_list[key] = i
		item.Set(key, i)
	}

	for ele := range item.Iterator(nil) {
		is.Equal(key_list[ele.Key()], ele.Value().(int))
	}
}

func TestCreateKVStoreCommit(t *testing.T) {
	is := is.New(t)
	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)
	db_path := tmp + "/test.db"
	test_db, err := db.Open(db_path)
	is.NoErr(err)

	err = test_db.CreateBucket("bucket")
	is.NoErr(err)

	item := db.NewKV()
	is.NotNil(item)

	item.Set("foo", "bar")
	v := item.Get("foo")
	is.Equal(v.(string), "bar")

	err = test_db.Commit("bucket", item.Snapshot())
	is.NoErr(err)

	test_db.Close()

	test_db2, err := db.Open(db_path)
	is.NoErr(err)
	defer test_db2.Close()

	for ele := range test_db2.Iterator("bucket", nil, nil) {
		is.Equal(ele.Key(), "foo")
		is.Equal(ele.Value(), "bar")
	}
}
