package fs_test

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/fs"
	"github.com/etcenter/c4/test"
)

func TestDuplicationReport(t *testing.T) {
	is := is.New(t)
	tmp := test.TempDir(is)
	defer test.DeleteDir(&tmp)
	threads := 8
	dup_rate := 35
	build_test_fs(is, tmp, 8, 20, uint32(dup_rate))
	f := fs.New(tmp)
	f.IdWorkers(threads)
	is.NotNil(f)
	// tmr := time.Now()
	ch := f.Walk()
	is.OK(ch)
	for n := range ch {
		is.NotNil(n)
	}
	f.Wait()
	f.IndexIds()
	dup_list := f.Duplication()
	size := int64(0)
	for _, nodes := range dup_list {
		if len(nodes) > 0 {
			size += int64(len(nodes)-1) * int64(nodes[0].Size())
		}
	}

	rate := float64(size) / float64(f.Size())
	rate_diff := math.Abs(float64(dup_rate) - (100 * rate))

	files, _ := f.Count()
	t.Log("Total files: ", files)
	t.Log("Duplicate files: ", len(dup_list))
	t.Log("Total Size: ", float64(f.Size())/(1024*1024), "MB")
	t.Log("Duplicate Size: ", float64(size)/(1024*1024), "MB")
	t.Log("Duplication Rate: ", rate)
	t.Log("Rate Error: ", rate_diff)

	is.True(f.Size() > 0)
	is.True(size > 0)
	is.True(f.Size() > size)
	is.True(rate_diff < 5)
}

func TestWalkFS(t *testing.T) {
	is := is.New(t)
	tmp := test.TempDir(is)
	defer test.DeleteDir(&tmp)
	threads := 8
	build_test_fs(is, tmp, 8, 20, 0)
	f := fs.New(tmp)
	is.NotNil(f)
	f.IdWorkers(threads)
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
	t.Log("file count:", files, "folder count:", folders)
	t.Log("threads:", threads, " time:", d)
}

func build_test_fs(is is.I, dir string, depth int, breadth int, duplication uint32) []string {
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

	duplicate_files := []string{}

	for _, d := range dir_list {
		for i := uint32(0); i < rand.Uint32()%10; i++ {
			file_name := fmt.Sprintf("%s/file_%04d.txt", d, rand.Uint32()%10000)
			fout, err := os.Create(file_name)
			is.NoErr(err)
			if (len(duplicate_files) == 0) || ((rand.Uint32() % 101) > duplication) {
				duplicate_files = append(duplicate_files, file_name)
				data := make([]byte, 4096*(rand.Uint32()%400))
				_, err = rand.Read(data)
				is.NoErr(err)
				fout.Write(data)
				fout.Close()
				continue
			}

			j := rand.Uint32() % uint32(len(duplicate_files))
			// fmt.Println("j:", j)
			fin, err := os.OpenFile(duplicate_files[j], os.O_RDONLY, 0777)
			is.NoErr(err)
			_, err = io.Copy(fout, fin)
			is.NoErr(err)
			fin.Close()
			fout.Close()
		}
	}
	return paths
}
