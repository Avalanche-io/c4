package attributes_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/cheekybits/is"
)

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
			defer f.Close()
			data := make([]byte, 4096)
			_, err = rand.Read(data)
			is.NoErr(err)
			f.Write(data)
		}
	}
	return paths
}

// func TestWalkFS(t *testing.T) {
// 	is := is.New(t)
// 	tmp := test.TempDir(is)
// 	defer test.DeleteDir(&tmp)
// 	build_test_fs(is, tmp, 4, 10)
// 	f := store.Walk(tmp)
// 	ch := f.Enqueue()
// 	for n := range ch {
// 		if n != nil {
// 			path := n.Path
// 			name := n.Label
// 			_ = path
// 			_ = name
// 			// fmt.Println(*path, *name)
// 		}
// 	}
// }
