package fs_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/fs"
	"github.com/etcenter/c4/test"
)

func TestWalkFS(t *testing.T) {
	is := is.New(t)
	tmp := test.TempDir(is)
	defer test.DeleteDir(&tmp)
	threads := 8
	build_test_fs(is, tmp, 8, 20)
	f := fs.New(tmp)
	f.IdWorkers(threads)
	is.NotNil(f)
	tmr := time.Now()
	ch := f.Walk()
	is.OK(ch)
	for n := range ch {
		is.NotNil(n)
	}
	f.Wait()
	d := time.Now().Sub(tmr)

	var idWalk func(node *fs.Node)
	idWalk = func(node *fs.Node) {
		is.NotNil(node.Id)
		// defer t.Log(node.Path(), ": ", node.Id)
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
	t.Log("size: ", f.Size())
	files, folders := f.Count()

	f.IndexIds()
	dup_list := f.Duplication()
	t.Log("Duplication: ", len(dup_list))
	for id, nodes := range dup_list {
		t.Log(id, ":")
		for i, n := range nodes {
			t.Log("\t", i, ": ", n.Path())
		}
	}
	t.Log("file count:", files, "folder count:", folders)
	t.Log("threads:", threads, " time:", d)
}

func build_test_fs(is is.I, dir string, depth int, breadth int) []string {
	var paths []string
	rand.Seed(0)
	for i := 0; i < breadth; i++ {
		d := []string{}
		for j := 0; j < depth; j++ {
			name := fmt.Sprintf("dir_%d_%d", i, j)
			d = append(d, name)
		}
		path := dir + string(filepath.Separator) + filepath.Join(d[:]...)
		paths = append(paths, path)
		err := os.MkdirAll(path, 0777)
		is.NoErr(err)
	}
	dir_list := []string{}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() || err != nil {
			return nil
		}
		dir_list = append(dir_list, path)
		return nil
	})

	for _, d := range dir_list {
		for i := uint32(0); i < rand.Uint32()%10; i++ {
			file_name := fmt.Sprintf("%s/file_%04d.txt", d, rand.Uint32()%10000)
			f, err := os.Create(file_name)
			is.NoErr(err)
			data := make([]byte, 4096*(rand.Uint32()%40))
			_, err = rand.Read(data)
			is.NoErr(err)
			f.Write(data)
			f.Close()
		}
	}
	return paths
}
