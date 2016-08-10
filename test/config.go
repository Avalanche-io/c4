package test

import (
	"io/ioutil"
	"os"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/env"
)

func TempDir(is is.I) string {
	dir, err := ioutil.TempDir("/tmp", "c4test_")
	is.NoErr(err)
	return dir
}

func DeleteDir(dir *string) {
	os.RemoveAll(*dir)
}

func TestConfig(is is.I) *env.Config {
	dir := TempDir(is)
	return env.NewConfig().WithRoot(dir)
}

func TestDeleteConfig(cfg *env.Config) {
	DeleteDir(cfg.Root)
}
