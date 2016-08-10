package env_test

import (
	"testing"

	"github.com/etcenter/c4/env"

	"github.com/cheekybits/is"
)

func TestMergeCfg(t *testing.T) {
	is := is.New(t)

	cfg1 := env.NewConfig()
	cfg2 := env.NewConfig().WithRoot("/tmp/test_path")
	cfg3 := env.NewConfig().WithRoot("/tmp/test_path2")
	cfg4 := env.NewConfig()

	// Merge overrides pre-existing value
	cfg1.Merge(cfg2)
	is.Equal(*cfg1, *cfg2)
	is.Equal(*cfg1.Root, "/tmp/test_path")

	cfg2.Merge(cfg3)
	is.Equal(*cfg2, *cfg3)
	is.Equal(*cfg2.Root, "/tmp/test_path2")

	// Unless the new value is null
	cfg3.Merge(cfg4)
	is.NotEqual(*cfg3, *cfg4)
	is.Equal(*cfg3.Root, "/tmp/test_path2")
}
