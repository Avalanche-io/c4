package store_test

import (
	"os"
	"path/filepath"
	"testing"

	store "github.com/Avalanche-io/c4/store"
)

func TestAssetSaveLoad(t *testing.T) {
	is, dir, done := SetupTestFolder(t, "store")
	defer done()

	path := filepath.Join(dir, "test_asset")

	f, err := os.Create(path)
	is.NoErr(err)
	f.WriteString("foo\n")
	f.Close()

	asset := store.NewAsset(path)
	is.NotNil(asset)
}
