package db_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/etcenter/c4/db"
	"github.com/etcenter/c4/test"

	"github.com/cheekybits/is"
)

func TestCreatesDB(t *testing.T) {
	is := is.New(t)
	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)

	db_path := tmp + "/c4.db"
	test_db, err := db.Open(db_path)
	is.NoErr(err)
	test_db.Close()
	if _, err = os.Stat(db_path); os.IsNotExist(err) {
		is.Fail("DB not created " + db_path)
	}
}

func TestCreatesBuckets(t *testing.T) {
	is := is.New(t)

	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)

	db_path := tmp + "/c4.db"
	test_db, err := db.Open(db_path)
	is.NoErr(err)

	bucketsIn := []string{"bucket1", "bucket2", "bucket3"}

	err = test_db.CreateBuckets(bucketsIn)
	is.NoErr(err)

	bucketsOut, err := test_db.ListBuckets()
	is.NoErr(err)

	for i, bucket := range bucketsOut {
		is.Equal(bucket, bucketsIn[i])
	}

}

func TestPut(t *testing.T) {
	is := is.New(t)

	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)

	db_path := tmp + "/c4.db"
	test_db, err := db.Open(db_path)
	is.NoErr(err)

	err = test_db.CreateBucket("bucket")
	is.NoErr(err)

	err = test_db.Put("bucket", []byte("key"), []byte("value: 42"))
	is.NoErr(err)
}

func TestGet(t *testing.T) {
	is := is.New(t)

	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)

	db_path := tmp + "/c4.db"
	test_db, err := db.Open(db_path)
	is.NoErr(err)

	err = test_db.CreateBucket("bucket")
	is.NoErr(err)

	value := "value: 42"
	err = test_db.Put("bucket", []byte("key"), []byte(value))
	is.NoErr(err)

	data, err := test_db.Get("bucket", []byte("key"))
	is.NoErr(err)

	is.Equal(data, []byte(value))
}

func TestIterate(t *testing.T) {
	is := is.New(t)

	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)

	db_path := tmp + "/c4.db"
	test_db, err := db.Open(db_path)
	is.NoErr(err)

	err = test_db.CreateBucket("bucket")
	is.NoErr(err)

	keys := [][]byte{
		[]byte("key1"),
		[]byte("key2"),
	}
	values := [][]byte{
		[]byte("value: 42"),
		[]byte("value: 42, Oh no, not again."),
	}

	err = test_db.Put("bucket", keys[0], values[0])
	is.NoErr(err)

	err = test_db.Put("bucket", keys[1], values[1])
	is.NoErr(err)

	i := 0
	test_db.Iterate("bucket", func(k []byte, v []byte) bool {
		is.Equal(k, keys[i])
		is.Equal(v, values[i])
		i++
		return true
	})
}

func TestIterator(t *testing.T) {
	is := is.New(t)
	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)

	db_path := tmp + "/c4.db"
	test_db, err := db.Open(db_path)
	is.NoErr(err)

	err = test_db.CreateBucket("bucket")
	is.NoErr(err)

	key_list := map[string]int{}
	var i int
	for i = 0; i < 1000; i++ {
		key := fmt.Sprintf("%08d", i)
		key_list[key] = i
		b, err := json.Marshal(i)
		is.NoErr(err)
		err = test_db.Put("bucket", []byte(key), b)
		is.NoErr(err)
	}
	for ele := range test_db.Iterator("bucket", nil, nil) {
		is.Equal(key_list[ele.Key()], ele.Value())
	}
}
