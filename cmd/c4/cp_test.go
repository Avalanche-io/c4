package main_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/test"

	c4 "github.com/etcenter/c4/cmd/c4"
)

func setup(is is.I) []string {
	var tempdirs []string

	target_count := 4
	for i := 0; i < target_count; i++ {
		t := test.TempDir(is)
		tempdirs = append(tempdirs, t)
	}
	build_test_fs(is, tempdirs[0], 8, 20, 0)

	return tempdirs
}

func teardown(is is.I, tempdirs []string) {
	for _, tmp := range tempdirs {
		test.DeleteDir(tmp)
	}
}

func TestAllCpFlags(t *testing.T) {
	is := is.New(t)
	tempdirs := setup(is)
	defer teardown(is, tempdirs)
	is.Equal(len(tempdirs), 4)
	srcdir := tempdirs[0]
	targets := tempdirs[1:]
	_ = srcdir
	_ = targets

	var cptests = []struct {
		flags  []string
		glob   []string
		target bool
		status int
	}{
		{[]string{}, []string{}, false, 64},
		{[]string{"-R"}, []string{"*"}, true, 0},
		{[]string{}, []string{"*.txt"}, true, 0},
		{[]string{}, []string{"*"}, true, 1},
	}

	os.Chdir(srcdir)

	for _, tt := range cptests {
		for _, dir := range targets {
			clean_temp_dir(is, dir)
		}
		// clean_temp_dir(is, targets[0])
		// clean_temp_dir(is, targets[1])

		flags := strings.Join(tt.flags, " ")
		globs := strings.Join(tt.glob, " ")
		var target string
		if tt.target {
			target = targets[0]
		}
		command := fmt.Sprintf("cp %s %s %s", flags, globs, target)

		echoCmd := exec.Command("/bin/sh", "-c", "echo "+command)
		var echoOut bytes.Buffer
		echoCmd.Stdout = &echoOut
		echoCmd.Run()

		cmd := exec.Command("/bin/sh", "-c", command)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		args := tt.flags
		files := []string{}
		for _, g := range tt.glob {
			f, err := filepath.Glob(g)
			is.NoErr(err)
			files = append(files, f...)
		}
		args = append(args, files...)
		if tt.target {
			args = append(args, targets[1])
		}

		// Test real cp'
		cp_err := cmd.Run()

		// Test c4 cp
		c4.CpFlags = c4.CpFlagsInit()
		err := c4.CpFlags.Parse(args)
		is.NoErr(err)
		// c4_out, c4_err :=
		stdoutch := make(chan string)
		stderrch := make(chan error)

		go func() {
			c4.CpMain(c4.CpFlags, stdoutch, stderrch)
			close(stdoutch)
			close(stderrch)
		}()
		var c4_stdout, c4_stderr bytes.Buffer
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			for err := range stderrch {
				_, err := c4_stderr.Write([]byte(err.Error()))
				is.NoErr(err)
			}
			wg.Done()
		}()
		go func() {
			for str := range stdoutch {
				_, err := c4_stdout.Write([]byte(str))
				is.NoErr(err)
			}
			wg.Done()
		}()
		wg.Wait()

		if cp_err != nil {
			// expected := fmt.Sprintf("exit status %d", tt.status)
			// is.Equal(expected, cp_err.Error())
			is.Equal(stderr.String(), c4_stderr.String())
		}
		is.Equal(stdout.String(), c4_stdout.String())
		ok := compare_folders(is, targets[0], targets[1])
		is.OK(ok)
	}
}

func clean_temp_dir(is is.I, dir string) {
	file, err := os.Open(dir)
	is.NoErr(err)
	files, err := file.Readdir(0)
	is.NoErr(err)
	for _, ff := range files {
		err := os.RemoveAll(dir + string(os.PathSeparator) + ff.Name())
		is.NoErr(err)
	}
}

func compare_folders(is is.I, cp_target string, c4_target string) bool {
	cp_map := make(map[string]string)
	c4_map := make(map[string]string)

	filepath.Walk(cp_target, func(path string, info os.FileInfo, err error) error {
		data := fmt.Sprintf("name:%q\tsize:%d\tmode:%s",
			info.Name(),
			info.Size(),
			info.Mode(),
		)
		key := path[len(cp_target):]
		cp_map[key] = data
		return nil
	})

	filepath.Walk(c4_target, func(path string, info os.FileInfo, err error) error {
		data := fmt.Sprintf("name:%q\tsize:%d\tmode:%s",
			info.Name(),
			info.Size(),
			info.Mode(),
		)
		key := path[len(c4_target):]
		c4_map[key] = data
		return nil
	})

	for k, v := range cp_map {
		if k == cp_target || k == "" {
			continue
		}
		if c4_map[k] != v {
			// fmt.Printf("failed comparison: %q\n", k)
			// fmt.Printf("\tcp_target: \t%s\n", filepath.Base(cp_target))
			// fmt.Printf("\tc4_target: \t%s\n", filepath.Base(c4_target))
			// fmt.Printf("  cp: \t%s\n", v)
			// fmt.Printf("  c4: \t%s\n", c4_map[k])
			return false
		}
		// fmt.Printf("\tcp_target: \t%s\n", filepath.Base(cp_target))
		// fmt.Printf("\tc4_target: \t%s\n", filepath.Base(c4_target))
		// fmt.Printf("\tcp: \n\t%s\n", v)
		// fmt.Printf("\tc4: \n\t%s\n", c4_map[k])
	}
	return true
}

// copied from c4/fs_test.go
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
				data := make([]byte, 4096*(rand.Uint32()%20))
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
