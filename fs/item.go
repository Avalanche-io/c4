package fs

import (
	"fmt"
	"os"
	"time"

	"github.com/Workiva/go-datastructures/trie/ctrie"

	"github.com/etcenter/c4/asset"
)

// type Item map[string]interface{}
type Item ctrie.Ctrie

type Entry struct {
	Key   string
	Value interface{}
}

func NewItem() *Item {
	return (*Item)(ctrie.New(nil))
}

func NewFileInfoItem(path string, info os.FileInfo) *Item {
	i := (*Item)(ctrie.New(nil))
	i.Set("id", nil)
	i.Set("name", info.Name())
	i.Set("path", path)
	i.Set("regular", !info.IsDir())
	i.Set("size", info.Size())
	i.Set("mode", info.Mode())
	i.Set("modetime", info.ModTime().UTC().Format(time.RFC3339))

	if info.IsDir() {
		p := (*Item)(ctrie.New(nil))
		p.Set(".", i)
		return p
	}
	return i
}

func (i *Item) IsNil(key string) bool {
	val, ok := (*ctrie.Ctrie)(i).Lookup([]byte(key))
	if !ok || val == nil {
		return true
	}
	return false
}

func (i *Item) GetOrCreate(key string, value interface{}) interface{} {
	if v := i.Get(key); v != nil {
		return v
	}

	i.Set(key, value)
	return value
}

func (i *Item) Set(key string, value interface{}) {
	(*ctrie.Ctrie)(i).Insert([]byte(key), value)
}

func (i *Item) Get(key string) interface{} {
	val, ok := (*ctrie.Ctrie)(i).Lookup([]byte(key))
	if ok {
		return val
	}
	return nil

}

func (i *Item) SetAttribute(key string, value interface{}) {
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	a.Set(key, value)
}

func (i *Item) GetAttribute(key string) interface{} {
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	return a.Get(key)
}

func (i *Item) Iterator(cancel <-chan struct{}) <-chan *Entry {
	out := make(chan *Entry)

	ch := (*ctrie.Ctrie)(i).Iterator(cancel)
	go func() {
		for e := range ch {
			ent := Entry{string(e.Key), e.Value}
			out <- &ent
		}
		close(out)
	}()

	return out
}

func (i *Item) Size() int64 {
	isDir := i.IsDir()
	a := i
	if isDir {
		a = i.Get(".").(*Item)
	}
	s := a.Get("size")
	if s != nil {
		// fmt.Fprintf(os.Stderr, "size %s: %d\n", i.Path(), s)
		return s.(int64)
	}

	size := int64(0)
	if isDir {
		// fmt.Fprintf(os.Stderr, "size %s: %d\n", i.Path(), s)
		for ele := range i.Iterator(nil) {
			if ele.Key == "." {
				continue
			}
			v := ele.Value.(*Item)
			size += v.Size()
		}
		a.Set("size", size)
		return size
	} else {
		// panic("Error: Item \"size\" unexpectedly null.")
		a.Set("size", -1)
	}
	return -1
}

func (i *Item) Id() *asset.ID {
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	id := a.Get("id")
	if id == nil {
		return nil
	}
	return id.(*asset.ID)
}

func (i *Item) Path() string {
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	return a.Get("path").(string)
}

// os.FileInfo interface implementation
func (i *Item) Name() string {
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	return a.Get("name").(string)
}

func (i *Item) Mode() os.FileMode {
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	return a.Get("mode").(os.FileMode)
}

func (i *Item) ModTime() time.Time {
	var timestr string
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	timestr = a.Get("modtime").(string)
	t, err := time.Parse(time.RFC3339, timestr)
	if err != nil {
		panic(err)
	}
	return t
}

func (i *Item) Sys() interface{} {
	a := i
	if i.IsDir() {
		a = i.Get(".").(*Item)
	}
	return a.Get("sys")
}

func (i *Item) IsDir() bool {
	if i.Get(".") == nil {
		return false
	}
	return true
}

func (i *Item) Print() {
	m := make(map[string]interface{})
	for ele := range i.Iterator(nil) {
		m[ele.Key] = ele.Value
	}
	fmt.Printf("%v\n", m)
}

// func (i Item) ResolveIds() int {
//   r := 0
//   if i.Id() == nil {
//     if n.Regular || n.Idable() {
//       n.Identify()
//       r = 1
//     } else {
//       return 0
//     }
//   }
//   p := n.Parent
//   if p != nil {
//     r += p.ResolveIds()
//   }
//   return r
// }
