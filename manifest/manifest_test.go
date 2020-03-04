package manifest_test

import (
	"bytes"
	"crypto/sha512"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/manifest"
	"github.com/absfs/memfs"
)

func TestManifest(t *testing.T) {

	filename := "test_manifest.c4m"
	// f, err := os.Create(filename)
	// if err != nil {
	// 	t.Errorf("error creating manifest %s", err)
	// }
	// defer os.Remove(filename)

	m := manifest.NewManifest()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Errorf("error getting absolute path for ... %s", err)
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		var id c4.ID
		if !info.IsDir() {
			var err error
			id, err = IdentifyFile(path)
			if err != nil {
				return err
			}
		}

		path = strings.TrimPrefix(path, root)
		// fmt.Printf("%s\n", path)
		fi := manifest.NewFileInfo(info, id)
		m.SetFileInfo(path, fi)

		return nil
	})
	if err != nil {
		t.Errorf("error walking filesystem %s", err)
	}
	// fmt.Printf("len m: %d\n", m.Len())
	// err = f.Close()
	if err != nil {
		t.Errorf("error closing manifest %s", err)
	}

	data, err := m.Marshal()
	if err != nil {
		t.Errorf("error marshaling manifest %s", err)
	}

	// fmt.Printf("%s\n", string(data))
	fout, err := os.Create(filename)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fout.Close()
	_, err = fout.Write(data)
	if err != nil {
		t.Fatal(err.Error())
	}

	m2 := manifest.NewManifest()
	m2.Unmarshal(bytes.NewReader(data))

	// fmt.Printf("len(m2): %d\n", m2.Len())
	data2, err := m2.Marshal()
	if err != nil {
		t.Errorf("error marshaling manifest #2 %s", err)
	}
	if bytes.Compare(data, data2) != 0 {
		t.Error("manifests are not identical")
	}

	// for _, path := range m2.Paths() {
	// 	info := m2.Get(path)
	// 	if info.IsDir() {
	// 		path += "/"
	// 	}
	// 	fmt.Printf("%s\n", path)
	// }
}

func TestParseFileInfo(t *testing.T) {
	input := "	-rw-r--r--    6148 2019-11-06T20:01:22Z .DS_Store                                         c458Yt9m2xPHH8jxfyipfqD9qsXpZh2fGD9HpbfwSFfAFgX9nWHQp1LG94SsEron2GteyvxfYmQcsUjvJCbxPuRTj6\n"
	info, err := manifest.ParseFileInfo(input)
	if err != nil {
		t.Error("unable to parse input " + err.Error())
	}
	if info == nil {
		t.Error("nil output")
		t.Fail()
	}
	if info.Mode().String() != "-rw-r--r--" {
		t.Error("wrong mode " + info.Mode().String())
	}

	if info.Size() != 6148 {
		t.Errorf("wrong size %d", info.Size())
	}
	if info.ModTime().Format(time.RFC3339) != "2019-11-06T20:01:22Z" {
		t.Errorf("wrong modtime %s", info.ModTime().Format(time.RFC3339))
	}
	if info.Name() != ".DS_Store" {
		t.Errorf("wrong name %q", info.Name())
	}

	if info.ID().String() != "c458Yt9m2xPHH8jxfyipfqD9qsXpZh2fGD9HpbfwSFfAFgX9nWHQp1LG94SsEron2GteyvxfYmQcsUjvJCbxPuRTj6" {
		t.Errorf("wrong c4 id %s", info.ID())
	}

}

func IdentifyFile(path string) (c4.ID, error) {
	var id c4.ID
	f, err := os.Open(path)
	if err != nil {
		return id, err
	}
	defer f.Close()

	h := sha512.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return id, err
	}
	copy(id[:], h.Sum(nil))
	return id, nil
}

func TestRamFs(t *testing.T) {

	m := manifest.NewManifest()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Errorf("error getting absolute path for ... %s", err)
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		var id c4.ID
		if !info.IsDir() {
			var err error
			id, err = IdentifyFile(path)
			if err != nil {
				return err
			}
		}

		path = strings.TrimPrefix(path, root)
		// fmt.Printf("%s\n", path)
		fi := manifest.NewFileInfo(info, id)
		m.SetFileInfo(path, fi)

		return nil
	})
	if err != nil {
		t.Errorf("error walking filesystem %s", err)
	}

	mfs, err := memfs.NewFS()
	if err != nil {
		t.Errorf("unable to create memfs %s", err)
	}
	paths := m.Paths()
	for _, path := range paths {
		if path == "" || path == "/" {
			continue
		}
		// fmt.Printf("%s\n", path)
		info := m.Get(path)
		if info.IsDir() {
			err := mfs.Mkdir(path, 0755)
			if err != nil {
				t.Errorf("failed to make dir %q %s", path, err)
				return
			}
			continue
		}
		f, err := mfs.Create(path)
		if err != nil {
			t.Errorf("failed to make file %q %s", path, err)
			return
		}
		_, err = f.Write([]byte(info.ID().String()))
		if err != nil {
			t.Errorf("failed to write to file %q %s", path, err)
			return
		}
		f.Close()
	}
	// f, err := mfs.Open("/")
	// if err != nil {
	// 	t.Errorf("failed to read ram file %s", err)
	// 	return
	// }
	// names, err := f.Readdirnames(-1)
	// if err != nil {
	// 	t.Errorf("failed to read file names %s", err)
	// 	return
	// }

	// for _, name := range names {
	// 	fmt.Printf("%s\n", name)
	// }
	// err = fstools.Walk(mfs, "/", func(path string, info os.FileInfo, err error) error {
	// 	fmt.Printf("MEM: %s\n", path)
	// 	return nil
	// })
	// if err != nil {
	// 	t.Errorf("failed to walk memfs %s", err)
	// 	return
	// }
}

/*
func TestStringSlice(t *testing.T) {
	var man manifest.Manifest

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("error finding absolute path %s", err)
	}

	t.Logf("root %q", root)
	i := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		p := filepath.ToSlash(path[len(root):])
		if len(p) == 0 {
			p = "/"
		}
		dir, name := filepath.Split(p)
		_, _ = dir, name

		// dir = filepath.Clean(dir)
		if info.IsDir() {
			name += "/"
		}
		// depth := 0
		// for _, c := range dir {
		// 	if c == filepath.Separator {
		// 		depth++
		// 	}
		// }

		// if dir == "/" {
		// 	depth = 0
		// }
		// indent := strings.Repeat("\t", depth)

		name = dir + name
		if name == "//" {
			name = "/"
		}
		list := []string{name, info.Mode().String(), strconv.Itoa(int(info.Size())), info.ModTime().UTC().Format(time.RFC3339)}

		man = append(man, strings.Join(list, " ")) //fmt.Sprintf("%s%s %d %s %s", indent, info.Mode(), info.Size(), info.ModTime().UTC().Format(time.RFC3339), name))
		i++
		return nil
	})
	if err != nil {
		t.Error(err)
	}
	lineList := make([]string, man.Len())
	copy(lineList, man)
	sort.Strings(lineList)
	sort.Sort(man)
	diffCount := 0
	for i, line := range man {
		if lineList[i] != line {
			diffCount++
		}
	}

	for _, line := range man[:100] {
		fmt.Println(line)
	}
	f, err := os.Create("test_out.txt")
	if err != nil {
		t.Fatalf("error creating output file %s", err)
	}
	defer f.Close()
	for _, line := range man {
		f.WriteString(line + "\n")
	}
	fmt.Printf("diffCount: %d\n", diffCount)
	fmt.Printf("filecount = %d\n", man.Len())
}
*/
