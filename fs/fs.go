package fs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/etcenter/c4/asset"
)

type Node struct {
	Regular  bool
	Name     string
	Parent   *Node
	Info     os.FileInfo
	Id       *asset.ID
	Children map[string]*Node
}

type FileSystem struct {
	Root    string
	IdChan  chan *Node
	IDwg    sync.WaitGroup
	Nodes   map[string]*Node
	IdIndex map[string][]*Node
}

func New(path string) *FileSystem {
	m := make(map[string]*Node)
	i := make(map[string][]*Node)
	fs := FileSystem{Root: path, Nodes: m, IdIndex: i}
	return &fs
}

func (f *FileSystem) Wait() {
	f.IDwg.Wait()
}

func (f *FileSystem) IndexIds() {
	var idWalk func(node *Node)
	idWalk = func(node *Node) {
		if node.Id != nil {
			c4 := node.Id.String()
			indexof := -1
			for i, n := range f.IdIndex[c4] {
				if n == node {
					indexof = i
					break
				}
			}
			if indexof < 0 {
				f.IdIndex[c4] = append(f.IdIndex[c4], node)
			}
		}

		if node.Children == nil {
			return
		}
		for _, n := range node.Children {
			idWalk(n)
		}
	}
	for _, v := range f.Nodes {
		idWalk(v)
	}
}

func (f *FileSystem) Duplication() map[string][]*Node {
	result := make(map[string][]*Node)
	for k, v := range f.IdIndex {
		if len(v) > 1 {
			result[k] = v
		}
	}
	return result
}

func (f *FileSystem) Size() int64 {
	size := int64(0)
	for _, n := range f.Nodes {
		size += n.Size()
	}
	return size
}

func (f *FileSystem) Count() (files int64, folders int64) {
	files = 0
	folders = 0
	for _, n := range f.Nodes {
		f, d := n.Count()
		files += f
		folders += d
	}
	return
}

func (f *FileSystem) IdWorkers(n int) chan<- *Node {
	if f.IdChan == nil {
		f.IdChan = make(chan *Node, 16)
	}

	f.IDwg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			var wg sync.WaitGroup
			for node := range f.IdChan {
				wg.Add(1)
				go func(node2 *Node) {
					node2.ResolveIds()
					wg.Done()
				}(node)
			}
			wg.Wait()
			f.IDwg.Done()
		}()
	}
	return f.IdChan
}

func (n *Node) ResolveIds() int {
	r := 0
	if n.Id == nil {
		if n.Regular || n.Idable() {
			n.Identify()
			r = 1
		} else {
			return 0
		}
	}
	p := n.Parent
	if p != nil {
		r += p.ResolveIds()
	}
	return r
}

func (n *Node) Path() string {
	p := n.Parent
	if p != nil {
		s := p.Path() + n.Name
		if !n.Regular {
			s += "/"
		}
		return s
	}
	return "/" + n.Name + "/"
}

func (n *Node) Size() int64 {
	if n.Regular && n.Info != nil {
		return n.Info.Size()
	} else {
		size := int64(0)
		for _, v := range n.Children {
			size += v.Size()
		}
		return size
	}
}

func (n *Node) Count() (files int64, folders int64) {
	files = 0
	folders = 0
	if n.Regular {
		files += 1
	} else {
		folders += 1
		for _, node := range n.Children {
			f, d := node.Count()
			files += f
			folders += d
		}
	}
	return
}

func (n *Node) Idable() bool {
	if n.Regular {
		if n.Id == nil {
			return false
		}
	} else {
		for _, v := range n.Children {
			if !v.Idable() {
				return false
			}
		}
	}
	return true
}

func (n *Node) Identify() {
	if n.Id != nil {
		return
	}
	if n.Regular {
		f, err := os.OpenFile(n.Path(), os.O_RDONLY, 0600)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		id, err := asset.Identify(f)
		if err != nil {
			panic(err)
		}
		n.Id = id
	} else {
		ids := asset.IDSlice{}
		for _, node := range n.Children {
			if node.Id == nil {
				return
				// panic("Identify")
			}
			ids.Push(node.Id)
		}
		id, err := ids.ID()
		if err != nil {
			panic(err)
		}
		n.Id = id
	}
	return
}

func NewNode(regular bool, name string, parent *Node) (node *Node) {
	if regular {
		n := Node{regular, name, parent, nil, nil, nil}
		node = &n
	} else {
		m := make(map[string]*Node)
		n := Node{regular, name, parent, nil, nil, m}
		node = &n
	}
	return
}

func (fs *FileSystem) MkChildren(dirs []string) *Node {
	m := fs.Nodes
	var p *Node
	p = nil
	for _, d := range dirs {
		if d == "" {
			continue
		}

		if m[d] == nil {
			m[d] = NewNode(false, d, p)
		}
		p = m[d]
		m = m[d].Children
	}
	return p
}

func (fs *FileSystem) Walk() <-chan *Node {
	ch := make(chan *Node)

	go func() {
		filepath.Walk(fs.Root, func(path string, info os.FileInfo, err error) error {
			dir, filename := filepath.Split(path)
			dirs := strings.Split(dir, string(filepath.Separator))
			p := fs.MkChildren(dirs)
			var n *Node
			n = p.Children[filename]
			if n == nil {
				n = NewNode(!info.IsDir(), filename, p)
				p.Children[filename] = n
			}
			n.Info = info
			if info.IsDir() {
				files, err := ioutil.ReadDir(path)
				if err != nil {
					return err
				}
				for _, f := range files {
					if n.Children[f.Name()] == nil {
						cn := NewNode(!f.IsDir(), f.Name(), n)
						n.Children[f.Name()] = cn
					}
				}
			}
			fs.IdChan <- n
			ch <- n
			return nil
		})
		close(fs.IdChan)
		close(ch)
	}()

	return ch
}
