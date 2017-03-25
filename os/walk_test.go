package os_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/vfs"

	c4os "github.com/Avalanche-io/c4/os"
)

func TestWalk(t *testing.T) {
	is, dir, done := setup(t, "os")
	defer done()
	tree, _ := makeFsTree(is)
	// fs := memfs.Create()
	item_count := tree(vfs.OS(), 8, 20, 0, dir, 0)
	cnt := 0
	err := c4os.Walk(dir, nil, func(path string, info c4os.FileInfo, err error) error {
		// fmt.Printf("%s: %s\n", info.ID(), path)
		cnt++
		return nil
	})
	is.NoErr(err)
	is.Equal(cnt, item_count)
}

func TestCopy(t *testing.T) {
	is, dir, _ := setup(t, "os")
	defer done()
	const A, B = 0, 1
	paths := make([]string, 2)
	paths[A] = filepath.Join(dir, "pathA")
	paths[B] = filepath.Join(dir, "pathB")
	err := os.MkdirAll(paths[A], 0700)
	is.NoErr(err)
	err = os.MkdirAll(paths[B], 0700)
	is.NoErr(err)

	tree, _ := makeFsTree(is)
	tree(vfs.OS(), 8, 20, 0, paths[A], 0)

	err = c4os.CopyFolder(paths[B], paths[A])
	is.NoErr(err)

}

// func TestCompare(t *testing.T) {
// 	is, dir, done := setup(t, "os")
// 	defer done()
// 	tree, _ := makeFsTree(is)
// 	item_count := tree(vfs.OS(), 8, 20, 0, dir, 0)
// 	cnt := 0
// 	dirs := []string{dir, dir}
// 	err := c4os.Compare(dirs, func(paths []string, info []c4os.FileInfo, err []error) []error {
// 		is.True(len(paths) == 2)
// 		is.Equal(paths[0], paths[1])
// 		is.Equal(info[0], info[1])
// 		cnt++
// 		return nil
// 	})
// 	is.NoErr(err)
// 	is.Equal(cnt, item_count)
// }

// func TestValueSort(t *testing.T) {
// 	is, dir, done := setup(t, "os")
// 	defer done()
// 	rand.Seed(time.Now().Unix())
// 	tree, _ := makeFsTree(is)
// 	// fs := memfs.Create()
// 	tree(vfs.OS(), 8, 20, 0, dir, 0)
// 	files1 := []string{}
// 	dirs1 := []string{}
// 	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
// 		if info.IsDir() {
// 			if path != dir {
// 				path = path + "/"
// 			}
// 			dirs1 = append([]string{path}, dirs1...)
// 			return nil
// 		}
// 		files1 = append([]string{path}, files1...)
// 		return nil
// 	})
// 	files2 := []string{}
// 	dirs2 := []string{}
// 	err := c4os.Walk(dir, &c4os.Options{c4os.ValueSort}, func(path string, info c4os.FileInfo, err error) error {
// 		if info.IsDir() {
// 			dirs2 = append(dirs2, path)
// 			return nil
// 		}
// 		files2 = append(files2, path)
// 		return nil
// 	})
// 	for i := 0; i < len(dirs1); i++ {
// 		s := dirs1[i]
// 		l := 60 - utf8.RuneCountInString(s)
// 		if l < 0 {
// 			l = 0
// 		}
// 		d1 := s + strings.Repeat(" ", l)
// 		fmt.Printf("%s %s\n", d1, dirs2[i])
// 	}
// 	fmt.Printf("\n")
// 	for i := 0; i < len(files1); i++ {
// 		s := files1[i]
// 		l := 60 - utf8.RuneCountInString(s)
// 		if l < 0 {
// 			l = 0
// 		}
// 		f1 := s + strings.Repeat(" ", l)
// 		fmt.Printf("%s %s\n", f1, files2[i])
// 	}
// 	is.NoErr(err)
// }

// func TestThreadedWalk(t *testing.T) {
// 	path := "../../../"

// 	// path := "objects"

// 	var listDir func(int, string) chan *pathID

// 	filejobs := make(chan *job)
// 	treejobs := make(chan *job)
// 	for i := 0; i < 4; i++ {
// 		go func(i int) {
// 			fmt.Printf("Starting file worker: %d\n", i)
// 			for job := range filejobs {
// 				e := c4.NewEncoder()
// 				f, _ := os.Open(*job.Path)
// 				_, _ = io.Copy(e, f)
// 				f.Close()
// 				dg := e.Digest()
// 				job.done <- &dg
// 				close(job.done)
// 				Finish(job)
// 			}
// 		}(i)
// 	}

// 	for i := 0; i < 8; i++ {
// 		go func(i int) {
// 			fmt.Printf("Starting tree worker: %d\n", i)
// 			for job := range treejobs {
// 				dg := (*job.Ids).Digest()
// 				job.done <- &dg
// 				close(job.done)
// 				Finish(job)
// 			}
// 		}(i)
// 	}

// 	listDir = func(depth int, path string) chan *pathID {
// 		dirout := make(chan *pathID)
// 		go func() {
// 			defer func() {
// 				close(dirout)
// 			}()
// 			// Open dir and get FileInfo for each item.
// 			f, _ := os.Open(path)
// 			infos, _ := f.Readdir(-1)
// 			info, _ := f.Stat()

// 			f.Close()

// 			children := make(chan *pathID)
// 			dirs := make(chan string) //*pathID
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
// 					fid := NewPathID(depth+1, newpath, info, filejobs, treejobs)
// 					fid.Close()
// 					children <- fid
// 				}
// 				close(dirs)
// 			}()
// 			go func() {
// 				// Handle child directories.
// 				for childpath := range dirs {
// 					for child := range listDir(depth+1, childpath) {
// 						// dirout <- child
// 						// pid.AddChild(child)
// 						children <- child
// 					}
// 				}
// 				close(children)
// 			}()
// 			pid := NewPathID(depth, path, info, filejobs, treejobs)
// 			for child := range children {
// 				pid.AddChild(child)
// 				dirout <- child
// 			}
// 			pid.Close()
// 			dirout <- pid
// 		}()
// 		return dirout
// 	}

// 	in := listDir(0, path)

// 	for dir := range in {
// 		if dir.Depth > 3 {
// 			Release(dir)
// 			continue
// 		}
// 		fmt.Print(dir.PerfString(path, -1))
// 		Release(dir)
// 	}
// 	close(filejobs)
// 	close(treejobs)
// }
