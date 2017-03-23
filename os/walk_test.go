package os

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	c4 "github.com/Avalanche-io/c4/id"
	// "github.com/cheekybits/is"
)

type IdInfoPath struct {
	Info os.FileInfo
	Id   *c4.ID
	Path string
}

type pathID struct {
	Depth       int
	Path        string
	dg          c4.Digest
	Info        os.FileInfo
	IdTime      time.Duration
	Size        uint64
	Count       uint64
	start       time.Time
	ids         c4.DigestSlice
	done        chan *c4.Digest
	fileworkers chan *job
	treeworkers chan *job
}

type job struct {
	Path *string
	Ids  *c4.DigestSlice
	done chan *c4.Digest
}

var jobPool = sync.Pool{
	New: func() interface{} {
		return new(job)
	},
}

func NewJob(path *string, ids *c4.DigestSlice, done chan *c4.Digest) *job {
	job := jobPool.Get().(*job)
	job.Path = path
	job.Ids = ids
	job.done = done
	return job
}

func Finish(job *job) {
	jobPool.Put(job)
}

var pathIDPool = sync.Pool{
	New: func() interface{} {
		return new(pathID)
	},
}

func Release(pid *pathID) {
	pathIDPool.Put(pid)
}

func NewPathID(depth int, path string, info os.FileInfo, fw chan *job, tw chan *job) *pathID {
	pid := pathIDPool.Get().(*pathID)
	pid.Depth = depth
	pid.Path = path
	pid.Info = info
	pid.start = time.Now()
	pid.done = make(chan *c4.Digest, 2)
	pid.fileworkers = fw
	pid.treeworkers = tw
	pid.Size = uint64(0)
	pid.Count = uint64(0)
	pid.IdTime = time.Duration(0)
	if info.IsDir() {
		pid.ids = c4.DigestSlice{}
	}
	return pid
}

func (p *pathID) Digest() c4.Digest {
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

func (p *pathID) Close() {
	if p.Info.IsDir() {
		if len(p.ids) > 8 {
			p.treeworkers <- NewJob(nil, &p.ids, p.done)
		} else {
			p.dg = p.ids.Digest()
			close(p.done)
		}
	} else if !p.IsRegular() {
		p.Size = uint64(0)
		p.Count = uint64(0)
		close(p.done)
	} else {
		p.fileworkers <- NewJob(&p.Path, nil, p.done)
		p.Size = uint64(p.Info.Size())
		p.Count = uint64(1)
	}
	p.IdTime = time.Now().Sub(p.start)
}

func (p *pathID) PerfString(path string, id_length int) string {
	id_str := "<nil>" + strings.Repeat(" ", 85)
	dg := p.Digest()
	if dg != nil {
		id_str = dg.ID().String()
	}
	s := p.IdTime.String()
	l := 15 - utf8.RuneCountInString(s)
	if l < 0 {
		l = 0
	}
	pTime := strings.Repeat(" ", l) + s
	s = FormattedSize(p.Size)
	l = 15 - utf8.RuneCountInString(s)
	if l < 0 {
		l = 0
	}
	pSize := strings.Repeat(" ", l) + s
	s = FormattedSize(uint64(p.DataRate()))
	l = 15 - utf8.RuneCountInString(s)
	if l < 0 {
		l = 0
	}
	pRate := strings.Repeat(" ", l) + s
	ap, _ := filepath.Abs(path)
	_, name := filepath.Split(ap)
	name += p.Path[len(path):]
	s = fmt.Sprintf("%d", p.Count)
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

func (p *pathID) AddChild(fid *pathID) {
	if fid.Depth != p.Depth+1 {
		return
	}
	dg := fid.Digest()
	p.ids.Insert(dg)
	p.Size += fid.Size
	p.Count += fid.Count
}

func TestThreadedWalk(t *testing.T) {
	path := "../../../"

	// path := "objects"

	var listDir func(int, string) chan *pathID

	filejobs := make(chan *job)
	treejobs := make(chan *job)
	for i := 0; i < 4; i++ {
		go func(i int) {
			fmt.Printf("Starting file worker: %d\n", i)
			for job := range filejobs {
				e := c4.NewEncoder()
				f, _ := os.Open(*job.Path)
				_, _ = io.Copy(e, f)
				f.Close()
				dg := e.Digest()
				job.done <- &dg
				close(job.done)
				Finish(job)
			}
		}(i)
	}

	for i := 0; i < 8; i++ {
		go func(i int) {
			fmt.Printf("Starting tree worker: %d\n", i)
			for job := range treejobs {
				dg := (*job.Ids).Digest()
				job.done <- &dg
				close(job.done)
				Finish(job)
			}
		}(i)
	}

	listDir = func(depth int, path string) chan *pathID {
		dirout := make(chan *pathID)
		go func() {
			defer func() {
				close(dirout)
			}()
			// Open dir and get FileInfo for each item.
			f, _ := os.Open(path)
			infos, _ := f.Readdir(-1)
			info, _ := f.Stat()

			f.Close()

			children := make(chan *pathID)
			dirs := make(chan string) //*pathID
			// If the folder isn't empty, then loop over the items
			go func() {
				for _, info := range infos {
					newpath := filepath.Join(path, info.Name())
					// if file is a directory add it to the directory list to handle later
					if info.IsDir() {
						// dirs = append(dirs, newpath+"/")
						dirs <- newpath + "/"
						continue
					}
					fid := NewPathID(depth+1, newpath, info, filejobs, treejobs)
					fid.Close()
					children <- fid
				}
				close(dirs)
			}()
			go func() {
				// Handle child directories.
				for childpath := range dirs {
					for child := range listDir(depth+1, childpath) {
						// dirout <- child
						// pid.AddChild(child)
						children <- child
					}
				}
				close(children)
			}()
			pid := NewPathID(depth, path, info, filejobs, treejobs)
			for child := range children {
				pid.AddChild(child)
				dirout <- child
			}
			pid.Close()
			dirout <- pid
		}()
		return dirout
	}

	in := listDir(0, path)

	for dir := range in {
		if dir.Depth > 3 {
			Release(dir)
			continue
		}
		fmt.Print(dir.PerfString(path, -1))
		Release(dir)
	}
	close(filejobs)
	close(treejobs)
}

func FormattedSize(size uint64) string {
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

func (p *pathID) DataRate() float64 {
	return float64(p.Size) / (float64(p.IdTime) / float64(time.Second))
}

func (p *pathID) IsDir() bool {
	return p.Info.IsDir()
}

func (p *pathID) IsRegular() bool {
	return p.Info.Mode().IsRegular()
}

func (p *pathID) ID() *c4.ID {
	return p.Digest().ID()
}
