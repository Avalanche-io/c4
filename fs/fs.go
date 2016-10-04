package fs

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"

	"github.com/etcenter/c4/asset"
)

type ColorFunc func(...interface{}) string

var (
	bold    ColorFunc
	red     ColorFunc
	yellow  ColorFunc
	green   ColorFunc
	blue    ColorFunc
	magenta ColorFunc
	cyan    ColorFunc
	white   ColorFunc
)

func init() {
	bold = color.New(color.Bold).SprintFunc()
	red = color.New(color.FgRed).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	green = color.New(color.FgGreen).SprintFunc()
	blue = color.New(color.FgBlue).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	cyan = color.New(color.FgCyan).SprintFunc()
	white = color.New(color.FgWhite).SprintFunc()
	_ = red
	_ = yellow
	_ = green
	_ = blue
	_ = magenta
	_ = cyan
	_ = white

}

type Node struct {
	Id      *asset.ID `json:"id"`
	Regular bool      `json:"regular"`
	Parent  *Node     `json:"-"`
	// os os.Info
	Name    string      `json:"-"`
	Size    int64       `json:"size"`
	Mode    uint32      `json:"mode"`
	ModTime time.Time   `json:"modtime"`
	IsDir   bool        `json:"-"`
	Sys     interface{} `json:"sys,omitempty"`
	// children if this is a folder
	Children NodeMap `json:"children,omitempty"`
}

type NodeMap map[string]*Node
type NodeSlice []*Node
type NodeSliceMap map[string]NodeSlice

// func (f *NodeMap) MarshalJSON() ([]byte, error) {
// 	data, err := json.Marshal(f.Id)
// 	if err != nil {
// 		return nil, err
// 	}
// 	output := []byte(`{"id":`)
// 	output = append(output, data...)
// 	output = append(output, []byte(`,`)...)
// 	info := osfileinfo{
// 		f.Info.Name(),
// 		f.Info.Size(),
// 		uint32(f.Info.Mode()),
// 		f.Info.ModTime().UTC(),
// 		f.Info.IsDir(),
// 		nil,
// 		// f.Info.Sys(),
// 	}
// 	data, err = json.Marshal(&info)
// 	if err != nil {
// 		return nil, err
// 	}
// 	output = append(output, []byte(`"info":`)...)
// 	output = append(output, data...)
// 	output = append(output, byte('}'))
// 	// fmt.Printf("data: %s\n", string(output))
// 	return output, nil
// }

// func (n *Node) MarshalJSON() ([]byte, error) {
// 	data, err := json.Marshal(n.Id)
// 	if err != nil {
// 		return nil, err
// 	}
// 	output := []byte(`{"id":`)
// 	output = append(output, data...)
// 	output = append(output, []byte(`,`)...)
// 	info := osfileinfo{
// 		f.Info.Name(),
// 		f.Info.Size(),
// 		uint32(f.Info.Mode()),
// 		f.Info.ModTime().UTC(),
// 		f.Info.IsDir(),
// 		nil,
// 		// f.Info.Sys(),
// 	}
// 	data, err = json.Marshal(&info)
// 	if err != nil {
// 		return nil, err
// 	}
// 	output = append(output, []byte(`"info":`)...)
// 	output = append(output, data...)
// 	output = append(output, byte('}'))
// 	// fmt.Printf("data: %s\n", string(output))
// 	return output, nil
// }

type FileSystem struct {
	Root    string
	IdChan  chan *Node
	IDwg    sync.WaitGroup
	Nodes   NodeMap
	IdIndex NodeSliceMap
}

func (fs *FileSystem) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(fs.Nodes)
	if err != nil {
		return nil, err
	}
	// output := []byte('[')
	// output = append(output, byte(']'))
	return data, nil
}

func New(path string) *FileSystem {
	m := make(NodeMap)
	i := make(NodeSliceMap)
	fs := FileSystem{Root: path, Nodes: m, IdIndex: i}
	return &fs
}

func (fs *FileSystem) Add(filenames ...string) <-chan *Node {
	ch := make(chan *Node)

	go func() {

		for _, f := range filenames {
			path := f
			dir, _ := filepath.Split(f)
			if dir == "" {
				path, err := filepath.Abs(f)
				if err != nil {
					panic(err)
				}
				dir, _ = filepath.Split(path)
			}
			if fs.Root == "" {
				fs.Root = dir
			}
			info, fserr := os.Stat(path)
			err := fs.Process(ch, path, info, fserr)
			if err != nil {
				panic(err)
			}
		}
		close(fs.IdChan)
		close(ch)
	}()

	return ch

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

func (f *FileSystem) Duplication() NodeSliceMap {
	result := make(NodeSliceMap)
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
		size += n.Size
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

// TODO: change to update parent size during walk
// func (n *Node) Size() int64 {
// 	if n.Regular && n.info != nil {
// 		return n.info.Size()
// 	} else {
// 		size := int64(0)
// 		for _, v := range n.Children {
// 			size += v.Size()
// 		}
// 		return size
// 	}
// }

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

// TODO: NewNodeFromInfo(info os.Info, parent *Node)
func NewNode(regular bool, name string, parent *Node) (node *Node) {
	if regular {
		// n := Node{nil, regular, name, parent, nil, nil}
		n := Node{nil, regular, parent, name, -1, 0, time.Time{}, !regular, nil, nil}
		node = &n
	} else {
		m := make(NodeMap)
		n := Node{nil, regular, parent, name, -1, 0, time.Time{}, !regular, nil, m}
		// n := Node{nil, regular, name, parent, nil, m}
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
		filepath.Walk(fs.Root, func(path string, info os.FileInfo, fserr error) error {
			return fs.Process(ch, path, info, fserr)
		})
		close(fs.IdChan)
		close(ch)
	}()

	return ch
}

func (fs *FileSystem) Process(ch chan<- *Node, path string, info os.FileInfo, fserr error) error {
	dir, filename := filepath.Split(path)
	if dir == "" {
		path, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		dir, filename = filepath.Split(path)
	}
	dirs := strings.Split(dir, string(filepath.Separator))
	p := fs.MkChildren(dirs)
	var n *Node
	n = p.Children[filename]
	if n == nil {
		n = NewNode(!info.IsDir(), filename, p)
		p.Children[filename] = n
	}
	// n.info = info

	if info.IsDir() {
		// n.Name = info.Name()
		n.Size = 0
		n.Mode = uint32(info.Mode())
		n.ModTime = info.ModTime()
		n.IsDir = true
		n.Sys = info.Sys()

		files, err := ioutil.ReadDir(path)
		if err != nil {
			return err
		}

		for _, f := range files {
			// n.Name = info.Name()
			if !f.IsDir() {
				n.Size += f.Size()
			}
			if n.Children[f.Name()] == nil {
				cn := NewNode(!f.IsDir(), f.Name(), n)
				n.Children[f.Name()] = cn
			}
		}
	} else {
		// n.Name = info.Name()
		n.Size = info.Size()
		n.Mode = uint32(info.Mode())
		n.ModTime = info.ModTime()
		n.IsDir = false
		n.Sys = info.Sys()
	}
	fs.IdChan <- n
	ch <- n
	return nil
}
