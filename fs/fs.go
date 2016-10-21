package fs

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type c4 string

type FileSystem struct {
	Root    string
	IdChan  chan *Item
	jsonW   io.Writer
	Nodes   *Item
	IdIndex map[c4][]string
}

func (fs *FileSystem) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(fs.Nodes)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func New(path string) *FileSystem {
	fs := FileSystem{
		Root:    path,
		Nodes:   NewItem(),
		IdIndex: map[c4][]string{},
	}
	return &fs
}

// func (fs *FileSystem) Add(filenames ...string) <-chan *Item {
// 	ch := make(chan *Item)

// 	go func() {
// 		for _, f := range filenames {
// 			info, fserr := os.Stat(f)
// 			if info.IsDir() {
// 				filepath.Walk(fs.Root+string(filepath.Separator)+f, func(path string, info os.FileInfo, fserr error) error {
// 					return fs.Process(ch, path, info, fserr)
// 				})
// 			} else {
// 				err := fs.Process(ch, fs.Root+string(filepath.Separator)+f, info, fserr)
// 				if err != nil {
// 					panic(err)
// 				}
// 			}
// 		}
// 		close(ch)
// 	}()
// 	return ch
// }

func (fs *FileSystem) Add(filenames ...string) error {
	for _, filename := range filenames {
		info, err := os.Stat(filename)
		if err != nil {
			return err
		}
		path, err := filepath.Abs(filename)
		if err != nil {
			return err
		}
		i := NewFileInfoItem(path, info)
		fs.Nodes.Set(info.Name(), i)
	}
	return nil
}

func (f *FileSystem) IndexIds() {
	var idWalk func(item Item)
	idWalk = func(item Item) {
		if item.Id() != nil {
			id := c4(item.Id().String())
			indexof := -1
			pathlist := f.IdIndex[id]
			for i, path := range pathlist {
				if string(path) == item.Path() {
					indexof = i
					break
				}
			}
			if indexof < 0 {
				f.IdIndex[id] = append(f.IdIndex[id], item.Path())
			}
		}

		if item.IsDir() {
			for ele := range item.Iterator(nil) {
				if ele.Key == "." {
					continue
				}
				idWalk(ele.Value.(Item))
			}
		}
	}
}

func (f *FileSystem) Duplication() *Item {
	result := NewItem()
	for k, v := range f.IdIndex {
		if len(v) > 1 {
			result.Set(string(k), v)
		}
	}
	return result
}

func (f *FileSystem) Size() int64 {
	size := int64(0)
	for ele := range f.Nodes.Iterator(nil) {
		i := ele.Value.(*Item)
		s := i.Size()
		size += s
	}
	return size
}

func (fs *FileSystem) Walk() <-chan *Item {
	ch := make(chan *Item)
	go func() {
		for ele := range fs.Nodes.Iterator(nil) {
			item := ele.Value.(*Item)
			if item.IsDir() {
				filepath.Walk(item.Path(), func(path string, info os.FileInfo, fserr error) error {
					return fs.Process(ch, path, info, fserr)
				})
			} else {
				ch <- item
			}
		}
		close(ch)
	}()

	return ch
}

func (fs *FileSystem) Process(ch chan<- *Item, path string, info os.FileInfo, fserr error) error {

	dir, _ := filepath.Split(path)

	if len(dir) <= len(fs.Root) {
		return nil
	}
	dir = dir[len(fs.Root):]
	dirs := strings.Split(dir, string(filepath.Separator))
	parent := fs.Nodes
	for _, d := range dirs[1 : len(dirs)-1] {
		parent = parent.Get(d).(*Item)
	}

	i := NewFileInfoItem(path, info)

	parent.Set(info.Name(), i)
	ch <- i
	return nil
}
