package os

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	c4 "github.com/Avalanche-io/c4/id"
)

type FileInfo interface {
	os.FileInfo
	ID() *c4.ID
	Digest() c4.Digest
	Path() string
	Time() time.Duration
}

type item struct {
	Depth int
	path  string
	dg    c4.Digest
	Info  os.FileInfo
	t     time.Duration
	size  uint64
	count uint64
	start time.Time
	ids   c4.DigestSlice
	err   error
	done  chan *c4.Digest
}

func (i item) ModTime() time.Time {
	return i.Info.ModTime()
}

func (i item) Name() string {
	return i.Info.Name()
}

func (i item) Size() int64 {
	return i.Info.Size()
}

func (i item) Sys() interface{} {
	return i.Info.Sys()
}

func (i item) Mode() os.FileMode {
	return i.Info.Mode()
}

var itemPool = sync.Pool{
	New: func() interface{} {
		return new(item)
	},
}

func release(pid *item) {
	itemPool.Put(pid)
}

func newItem(depth int, path string, info os.FileInfo) *item {
	pid := itemPool.Get().(*item)
	pid.Depth = depth
	pid.path = path
	pid.Info = info
	pid.start = time.Now()
	pid.done = make(chan *c4.Digest, 2)
	pid.size = uint64(0)
	pid.count = uint64(0)
	pid.t = time.Duration(0)
	if info.IsDir() {
		pid.ids = c4.DigestSlice{}
	}
	return pid
}

func (p *item) Path() string {
	return p.path
}

func (p *item) Time() time.Duration {
	return p.t
}

func (p *item) DataRate() float64 {
	return float64(p.size) / (float64(p.t) / float64(time.Second))
}

func (p *item) IsDir() bool {
	return p.Info.IsDir()
}

func (p *item) IsRegular() bool {
	return p.Info.Mode().IsRegular()
}

func (p *item) ID() *c4.ID {
	if p.Digest() == nil {
		return nil
	}
	return p.Digest().ID()
}

func (p *item) Digest() c4.Digest {
	var dg *c4.Digest
	ok := true
	for ok {
		select {
		case dg, ok = <-p.done:
			if ok {
				p.dg = *dg
			}
		}
	}
	return p.dg
}

func (p *item) Close() {
	if p.Info.IsDir() {
		if len(p.ids) > 8 {
			Worker <- NewJob(nil, &p.ids, p.done)
		} else {
			p.dg = p.ids.Digest()
			close(p.done)
		}
	} else if !p.IsRegular() {
		p.size = uint64(0)
		p.count = uint64(0)
		close(p.done)
	} else {
		Worker <- NewJob(&p.path, nil, p.done)
		p.size = uint64(p.Info.Size())
		p.count = uint64(1)
	}
	p.t = time.Now().Sub(p.start)
}

func (p *item) AddChild(fid *item) {
	if fid.Depth != p.Depth+1 {
		return
	}
	dg := fid.Digest()
	p.ids.Insert(dg)
	p.size += fid.size
	p.count += fid.count
}

func (p *item) perfString(path string, id_length int) string {
	id_str := "<nil>" + strings.Repeat(" ", 85)
	dg := p.Digest()
	if dg != nil {
		id_str = dg.ID().String()
	}
	s := p.t.String()
	l := 15 - utf8.RuneCountInString(s)
	if l < 0 {
		l = 0
	}
	pTime := strings.Repeat(" ", l) + s
	s = formattedsize(p.size)
	l = 15 - utf8.RuneCountInString(s)
	if l < 0 {
		l = 0
	}
	pSize := strings.Repeat(" ", l) + s
	s = formattedsize(uint64(p.DataRate()))
	l = 15 - utf8.RuneCountInString(s)
	if l < 0 {
		l = 0
	}
	pRate := strings.Repeat(" ", l) + s
	ap, _ := filepath.Abs(path)
	_, name := filepath.Split(ap)
	name += p.path[len(path):]
	s = fmt.Sprintf("%d", p.count)
	l = 9 - utf8.RuneCountInString(s)
	if l < 0 {
		l = 0
	}
	pCount := strings.Repeat(" ", l) + s
	if id_length < 0 || id_length > 90 {
		id_length = 90
	}
	return fmt.Sprintf("%s: %d,\t%s,\t%s,\t%s/s: %s %s \n", pCount, p.Depth, pTime, pSize, pRate, id_str[:id_length], name)
}

type Options struct {
	Sorting Sorting
}

var DefaultOptions Options = Options{Unsorted}

func Walk(path string, opts *Options, f func(path string, info FileInfo, err error) error) error {
	if Worker == nil {
		Worker = make(chan *job)
	}
	if opts == nil {
		opts = &DefaultOptions
	}

	go func() {
		for job := range Worker {
			if job.path == nil {
				dg := (*job.Ids).Digest()
				job.done <- &dg
				close(job.done)
				finish(job)
			}
			e := c4.NewEncoder()
			f, _ := os.Open(*job.path)
			_, _ = io.Copy(e, f)
			f.Close()
			dg := e.Digest()
			job.done <- &dg
			close(job.done)
			finish(job)
		}
	}()

	stop := make(chan struct{})
	ch := walk(0, path, opts, stop)

	for info := range ch {
		err := f(info.path, info, info.err)
		if err != nil {
			close(stop)
		}
	}
	close(Worker)
	Worker = nil
	return nil
}

func walk(depth int, path string, opts *Options, stop chan struct{}) chan *item {
	out := make(chan *item)
	go func() {

		defer func() {
			close(out)
		}()
		// Open dir and get FileInfo for each item.
		f, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		infos, err := f.Readdir(-1)
		if err != nil {
			panic(err)
		}
		info, err := f.Stat()
		if err != nil {
			panic(err)
		}

		f.Close()

		list := randFileInfoList(infos)
		sort.Sort(list)

		children := make(chan *item)
		dirs := make(chan string)
		// If the folder isn't empty, then loop over the items
		go func() {
			defer close(dirs)

			// This feels way too heavy handed. Due for improvement.
			var list Sortable
			switch opts.Sorting {
			case NaturalSort:
				list = naturalFileInfoList(infos)
			case LexSort:
				list = lexFileInfoList(infos)
			case ValueSort:
				list = valueFileInfoList(infos)
			case Unsorted:
				fallthrough
			default:
				list = nilSort(infos)
			}
			sort.Sort(list)

			for _, info := range infos {
				newpath := filepath.Join(path, info.Name())
				// if file is a directory add it to the directory list to handle later
				if info.IsDir() {
					select {
					case dirs <- newpath + "/":
					case <-stop:
						return
					}
					continue
				}
				fid := newItem(depth+1, newpath, info)
				fid.Close()
				select {
				case children <- fid:
				case <-stop:
					return
				}
			}
		}()
		go func() {
			defer close(children)
			for childpath := range dirs {
				for child := range walk(depth+1, childpath, opts, stop) {
					select {
					case children <- child:
					case <-stop:
						return
					}
				}
			}
		}()
		pid := newItem(depth, path, info)
		for child := range children {
			pid.AddChild(child)
			select {
			case out <- child:
			case <-stop:
				return
			}

		}
		pid.Close()
		select {
		case out <- pid:
		case <-stop:
			return
		}
	}()
	return out
}

// var Walk func(int, string) chan *item

// func init() {
// 	Walk = func(depth int, path string) chan *item {
// 		out := make(chan *item)
// 		go func() {
// 			defer func() {
// 				close(out)
// 			}()
// 			// Open dir and get FileInfo for each item.
// 			f, _ := os.Open(path)
// 			infos, _ := f.Readdir(-1)
// 			info, _ := f.Stat()

// 			f.Close()

// 			children := make(chan *item)
// 			dirs := make(chan string) //*item
// 			// If the folder isn't empty, then loop over the items
// 			go func() {
// 				for _, info := range infos {
// 					newpath := filepath.Join(path, info.Name())
// 					// if file is a directory add it to the directory list to handle later
// 					if info.IsDir() {
// 						// dirs = append(dirs, newpath+"/")
// 						dirs <- newpath + "/"
// 						continue
// 					}
// 					fid := newItem(depth+1, newpath, info, filejobs, treejobs)
// 					fid.Close()
// 					children <- fid
// 				}
// 				close(dirs)
// 			}()
// 			go func() {
// 				// Handle child directories.
// 				for childpath := range dirs {
// 					for child := range Walk(depth+1, childpath) {
// 						// out <- child
// 						// pid.AddChild(child)
// 						children <- child
// 					}
// 				}
// 				close(children)
// 			}()
// 			pid := newItem(depth, path, info, filejobs, treejobs)
// 			for child := range children {
// 				pid.AddChild(child)
// 				out <- child
// 			}
// 			pid.Close()
// 			out <- pid
// 		}()
// 		return out
// 	}

// 	in := listDir(0, path)

// 	for dir := range in {
// 		if dir.Depth > 3 {
// 			release(dir)
// 			continue
// 		}
// 		fmt.Print(dir.PerfString(path, -1))
// 		release(dir)
// 	}
// }

func formattedsize(size uint64) string {
	if size == 0 {
		return "0  b"
	}
	sizes := []string{" b", "KB", "MB", "GB", "TB", "PB"}
	k := uint64(1024)
	for _, s := range sizes {
		if size < k {
			k = k / 1024
			return fmt.Sprintf("%.2f %s", float64(size)/float64(k), s)
		}
		k *= 1024
	}
	return "inf"
}
