package fs_test

import (
	"fmt"
	"testing"
	"time"

	// "github.com/Workiva/go-datastructures/trie/ctrie"

	"github.com/cheekybits/is"

	// "github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/fs"
	// "github.com/etcenter/c4/test"
)

func TestCreateItem(t *testing.T) {
	is := is.New(t)
	start := time.Now()
	fmt.Println("TestCreateItem")
	item := fs.NewItem()
	is.NotNil(item)
	timer := time.Now().Sub(start)
	fmt.Println("timer:", timer)
}
