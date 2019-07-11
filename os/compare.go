package os

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// updated, delete me

type CompareFunc func(paths []string, info []FileInfo, err []error) []error

// stringsDiff returns a slice of int which index the members of a that are not
// members of b.
func stringsDiff(a, b []string) []int {
	out := []int{}
	m := make(map[string]int)
	for i, s := range b {
		m[s] = i
	}

	for i, s := range a {
		_, ok := m[s]
		if !ok {
			out = append(out, i)
		}
	}
	return out
}

func stringsUnion(a, b []string) []int {
	out := []int{}
	m := make(map[string]int)
	for i, s := range b {
		m[s] = i
	}

	for i, s := range a {
		_, ok := m[s]
		if ok {
			out = append(out, i)
		}
	}
	return out
}

func CopyFolder(dest, src string) error {
	var srcNames []string
	var destNames []string

	srcF, err := os.Open(src)
	if err != nil {
		return err
	}

	destF, err := os.Open(dest)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		err = os.Mkdir(dest, 0700)
		if err != nil {
			return err
		}
		destF, err = os.Open(dest)
		if err != nil {
			return err
		}
	}

	srcNames, err = srcF.Readdirnames(-1)
	if err != nil && err != io.EOF {
		return err
	}
	srcF.Close()
	destNames, err = destF.Readdirnames(-1)
	if err != nil && err != io.EOF {
		return err
	}
	destF.Close()

	diff := stringsDiff(srcNames, destNames)
	union := stringsUnion(srcNames, destNames)
	dirs := make(map[int]os.FileInfo)
	var path string
	var srcInfo, destInfo os.FileInfo
	for i, name := range srcNames {
		path = filepath.Join(src, name)
		srcInfo, err = os.Stat(path)
		if err != nil {
			return err
		}
		if srcInfo.IsDir() {
			dirs[i] = srcInfo
		}
	}
	for _, i := range union {
		path = filepath.Join(src, srcNames[i])
		srcInfo, err = os.Stat(path)
		if err != nil {
			return err
		}
		path = filepath.Join(dest, srcNames[i])
		destInfo, err = os.Stat(path)
		if err != nil {
			return err
		}
		if compareFileInfo(destInfo, srcInfo) != 0 {
			diff = append(diff, i)
		}
	}
	for _, i := range diff {
		source := filepath.Join(src, srcNames[i])
		destination := filepath.Join(dest, srcNames[i])
		if dirs[i] != nil {
			err = CopyFolder(destination, source)
		} else {
			err = CopyFile(destination, source)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func compareFileInfo(srcInfo, destInfo os.FileInfo) int {
	fmt.Printf("compareFileInfo %s, %s\n", srcInfo.Name(), destInfo.Name())
	return 0
}

func CopyFile(dest, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer in.Close()
	// TODO: grab ID
	io.Copy(out, in)
	info, err := in.Stat()
	if err != nil {
		return err
	}
	err = out.Chmod(info.Mode())
	if err != nil {
		return err
	}
	return nil
}
