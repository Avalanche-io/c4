package os

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	// "github.com/Avalanche-io/c4/events"
	c4id "github.com/Avalanche-io/c4/id"
	c4time "github.com/Avalanche-io/c4/time"

	"github.com/blang/vfs"
)

type FileSystem struct {
	fs    vfs.Filesystem
	root  []byte
	attrs *FileSystemAttributes
	// events events.Chan
}

func NewFileSystem(vfs vfs.Filesystem, root []byte) *FileSystem {
	if root[len(root)-1] != "/"[0] {
		root = append(root, "/"[0])
	}
	attrs := FileSystemAttributes(make(map[string]Attributes))
	// fs := &FileSystem{vfs, root, &attrs, make(events.Chan)}
	fs := &FileSystem{vfs, root, &attrs}

	return fs
}

type FileSystemAttributes map[string]Attributes

type dot struct {
	name     *string
	Id       *c4id.ID    `json:"id"`
	SizeV    *big.Int    `json:"size"`
	ModeV    os.FileMode `json:"mode"`
	ModTimeV c4time.Time `json:"modtime"`
	isdir    bool
	sys      interface{}
}

func (d *dot) String() string {
	return fmt.Sprintf("*c4.dot=&{Name:%s, Id:%s, Size:%s, Mode:%s, Time:%s, IsDir:%t}",
		*d.name,
		d.Id,
		d.SizeV,
		d.ModeV,
		d.ModTimeV,
		d.isdir)
}

func (d *dot) Name() string {
	return *d.name
}

func (d *dot) Size() int64 {
	return d.SizeV.Int64()
}

func (d *dot) Mode() os.FileMode {
	return d.ModeV
}

func (d *dot) ModTime() time.Time {
	return d.ModTimeV.AsTime()
}

func (d *dot) IsDir() bool {
	return d.isdir
}

func (d *dot) Sys() interface{} {
	return d.sys
}

func (d *dot) Info(info os.FileInfo) *dot {
	if !info.IsDir() {
		d.SizeV = big.NewInt(info.Size())
	}
	d.ModTimeV = c4time.NewTime(info.ModTime())
	d.ModeV = info.Mode()
	d.sys = info.Sys()
	d.isdir = info.IsDir()
	name := info.Name()
	d.name = &name
	return d
}

func (fs *FileSystem) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	root := string(fs.root)
	m[root] = make(map[string]interface{})
	mm := m
	for k, v := range *fs.attrs {
		key := string(k)
		if root != "/" {
			key = strings.Replace(key, root, "", -1)
		}
		key = filepath.Clean(key)
		keys := strings.Split(key, string(filepath.Separator))
		id := v.ID()
		info, err := v.Stat()
		if err != nil {
			return nil, err
		}
		d := info.(*dot)
		d.Id = id
		m = mm[root].(map[string]interface{})
		for i, kk := range keys {
			if kk == "." {
				break
			}
			if kk == "" {
				continue
			}
			if i < len(keys)-1 || info.IsDir() {
				kk += "/"
			}
			_, ok := m[kk]
			if !ok {
				m[kk] = make(map[string]interface{})
			}
			m = m[kk].(map[string]interface{})
		}
		// info, err := v.Stat()
		// if err != nil {
		// 	return nil, err
		// }
		// _, ok := m["."]
		// if !ok {
		// 	m["."] = make(map[string]interface{})
		// }
		// m["."].(map[string]interface{})["info"] = FileInfo{info}

		m["."] = d
		// id, err := v.ID()
		// if err != nil {
		// 	return nil, err
		// }
		// m["."].(map[string]interface{})["id"] = id
	}
	return json.Marshal(mm)
}

func (fs *FileSystem) String() string {
	return fmt.Sprintf("*c4.FileSystem=&{%s %v}", string(fs.root), fs.attrs)
}

func (f *FileSystem) Keys() chan string {
	out := make(chan string)
	go func() {
		for k, _ := range *f.attrs {
			out <- k
		}
		close(out)
	}()
	return out
}

func (f *FileSystem) Attriubtes(path []byte) Attributes {
	kv := f.attrs
	a, ok := (*kv)[string(path)]
	if ok {
		return a
	}
	fsa := fs_attributes{}
	fsa.fs = f
	fsa.prefix = string(path)
	(*kv)[string(path)] = &fsa
	return &fsa
}

type fs_attributes struct {
	prefix string
	fs     *FileSystem
	id     *c4id.ID
	info   *dot
}

func (a *fs_attributes) String() string {
	return fmt.Sprintf("*c4.fs_attributes=&{%s %s}", a.id, a.info)
}

func (a *fs_attributes) Set(key []byte, v interface{}) error {
	if string(key) == "id" {
		if v == nil {
			a.id = nil
			return nil
		}
		a.id = v.(*c4id.ID)
		return nil
	}
	if string(key) == "info" {
		a.info = v.(*dot)
		return nil
	}
	return nil
}

func (a *fs_attributes) Get(key []byte, v interface{}) error {
	if string(key) == "id" {
		v = a.id
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return errors.New("argument must be a pointer")
	} else if rv.IsNil() {
		return errors.New("argument is nil")
	}
	if string(key) == "info" {
		rv.Elem().Set(reflect.ValueOf(a.info))
		return nil
	}
	path := filepath.Join(a.prefix + string(key))
	v = a.fs.Attriubtes([]byte(path))
	return nil
}

func (a *fs_attributes) Delete(key []byte) (interface{}, bool) {
	// NoOp at the moment.
	// m, abs := a.map_path(key)
	// v, ok := m[string(abs)]
	// if ok {
	// 	delete(m, string(abs))
	// }
	// return v, ok
	return a, true
}

func (a *fs_attributes) ForEach(prefix string, f func(key []byte, v interface{}) error) error {
	// id
	err := f([]byte("id"), a.id)
	if err != nil {
		return err
	}
	// info
	err = f([]byte("info"), a.info)
	if err != nil {
		return err
	}
	p := filepath.Join(a.prefix, prefix)
	p += "/"
	for path, v := range *a.fs.attrs {
		// FIX: very unoptimized.
		// Because we are iterating over map we must
		// filter on p (the combined prefixes)
		if strings.Index(path, p) == -1 {
			continue
		}
		// ForEach is not called on the item itself.
		if len(path) == len(p) {
			continue
		}
		err = f([]byte(path), v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *fs_attributes) Stat() (os.FileInfo, error) {
	return a.info, nil
}

func (a *fs_attributes) ID() *c4id.ID {
	return a.id
}

type AttributeFunc func(key []byte, attrs Attributes) error

func (fs *FileSystem) IdFile(filename string) (*c4id.ID, error) {
	f, err := fs.fs.OpenFile(filename, os.O_RDONLY, 0700)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return c4id.Identify(f)
}

func mustStat(info os.FileInfo, err error) os.FileInfo {
	if err != nil {
		panic(err)
	}
	return info
}

func (fs *FileSystem) Walk(name []byte, f AttributeFunc) error {
	abs := filepath.Join(string(fs.root), string(name))
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	_, _, err = fs.descend(abs, info, f)
	return err
}

func mustStep(id *c4id.ID, err error) *c4id.ID {
	if err != nil {
		panic(err)
	}
	return id
}

var infoK []byte = []byte("info")
var idK []byte = []byte("id")

func (fs *FileSystem) ReadDir(key []byte) ([]os.FileInfo, error) {
	return fs.fs.ReadDir(string(key))
}

func (fs *FileSystem) set_attr(key []byte, info os.FileInfo) ([]byte, Attributes, *big.Int) {
	if info.IsDir() {
		key = append(key, '/')
	}
	// creates a new 'fs_attributes' at key if one doesn't exist
	at := fs.Attriubtes(key)
	d := &dot{}
	size := big.NewInt(0)
	d.SizeV = size
	at.Set(infoK, d.Info(info))
	return key, at, size
}

// ascend walks up a tree adding files and folders to the attribute store.
func (fs *FileSystem) ascend(path string, info os.FileInfo) {
	root := string(fs.root)
	splitter := path
	if len(root) > 0 && root != "/" {
		if strings.Index(splitter, root) == 0 {
			splitter = path[len(root):]
		}
	}
	if !info.IsDir() {
		splitter = filepath.Dir(splitter)
	}

	folders := strings.Split(splitter, "/")
	path = root
	type id_stack struct {
		Path string
		At   Attributes
	}
	stack := []id_stack{}
	for _, folder := range folders {
		path = filepath.Join(path, folder)
		path += "/"
		// fmt.Printf("%d: folder: %s\n", i, path)
		at := fs.Attriubtes([]byte(path)) // insure it exists, but don't need make changes
		stack = append([]id_stack{{path, at}}, stack...)
	}
	for _, ele := range stack {
		// fmt.Printf("root: %s, path: %s\n", root, ele.Path)
		ele.At.ForEach("", func(key []byte, v interface{}) error {
			switch val := v.(type) {
			case *dot:
				// fmt.Printf("dot: %s\n", val)
			case *c4id.ID:
				// fmt.Printf("key: %s, v.(type): %T\n", key, v)
			case Attributes:
				fmt.Printf("key: %s, Attributes: %s\n", key, val)
			default:
				fmt.Printf("key: %s, v.(type): %T\n", key, val)
			}
			return nil
		})

		// key := ele.Path
		// for _, child := range mustReadDir(fs.ReadDir(key)) {
		// 	p := filepath.Join(path, child.Name())
		// }
	}
}

// descend walks down a tree adding files and folders to the attribute store.
func (fs *FileSystem) descend(path string, info os.FileInfo, f AttributeFunc) (*c4id.ID, *big.Int, error) {
	key, at, size := fs.set_attr([]byte(path), info)

	if !info.IsDir() {
		id, err := fs.IdFile(path)
		if err != nil {
			return nil, size, err
		}
		at.Set(idK, id)
		err = f(key, at)
		if err != nil {
			return id, size, err
		}
		return id, size, nil
	}

	var ids c4id.IDSlice
	dirs, err := fs.ReadDir(key)
	for _, child := range dirs {
		p := filepath.Join(path, child.Name())
		id, s, err := fs.descend(p, child, f)
		if err != nil {
			return id, s, err
		}
		if id == nil {
			continue
		}
		size.Add(size, s)
		ids.Push(id)
	}
	if ids.Len() == 0 {
		at.Set(idK, nil)
		err := f(key, at)
		return nil, size, err
	}
	id, err := ids.ID()
	if err != nil {
		return nil, size, err
	}
	at.Set(idK, id)
	err = f(key, at)
	return id, size, err
}
