package store_test

import (
	"os"
	"path/filepath"
	"testing"

	store "github.com/Avalanche-io/c4/store"
)

// updated, delete me

func TestAssetSaveLoad(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()

	path := filepath.Join(dir, "test_asset")

	f, err := os.Create(path)
	is.NoErr(err)
	f.WriteString("foo\n")
	f.Close()

	asset, err := store.NewFileAsset(path, nil, os.O_RDWR, nil, nil)
	is.NoErr(err)
	is.NotNil(asset)
}
