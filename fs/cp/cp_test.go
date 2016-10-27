package cp_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/test"

	"github.com/etcenter/c4/client"
	c4 "github.com/etcenter/c4/fs/cp"
)

func setup(is is.I, count int) []string {
	var tempdirs []string

	temp_count := count
	for i := 0; i < temp_count; i++ {
		t := test.TempDir(is)
		tempdirs = append(tempdirs, t)
	}
	test.TestFs(is, tempdirs[0], 3, 5, 0)

	return tempdirs
}

func teardown(is is.I, tempdirs []string) {
	for _, tmp := range tempdirs {
		test.DeleteDir(tmp)
	}
}

// TestCPFlags evaluates the build in 'cp' command with various flags
// and insures that the c4 cp function has the same output, and effect.
// TODO: currently only working for os x, it needs to switch based on OS.
func TestCpFlags(t *testing.T) {
	is := is.New(t)
	tempdirs := setup(is, 3)
	defer teardown(is, tempdirs)
	is.Equal(len(tempdirs), 3)
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
		{[]string{}, []string{"*"}, true, 1},
		{[]string{}, []string{"*.txt"}, true, 0},
		{[]string{"-R"}, []string{"*"}, true, 0},
		{[]string{"-Rv"}, []string{"*"}, true, 0},
	}

	os.Chdir(srcdir)
	cp_target := targets[0]
	c4_target := targets[1]

	for _, tt := range cptests {
		for _, dir := range targets {
			clean_temp_dir(is, dir)
		}
		clean_temp_dir(is, cp_target)
		clean_temp_dir(is, c4_target)

		flag_str := strings.Join(tt.flags, " ")
		glob_str := strings.Join(tt.glob, " ")
		var target string
		var stdout, stderr bytes.Buffer

		args := tt.flags
		args = append(args, build_file_list(is, tt.glob)...)

		if tt.target {
			target = cp_target
			args = append(args, c4_target)
		}

		// Test real cp'
		cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cp %s %s %s", flag_str, glob_str, target))
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cp_err := cmd.Run()

		// Test c4 cp

		client.CpFlags = client.CpFlagsInit()
		err := client.CpFlags.Parse(args)
		is.NoErr(err)

		// c4_out, c4_err :=
		c4_stdoutch := make(chan string, 1)
		c4_stderrch := make(chan error, 1)
		var c4_stderr, c4_stdout []string

		io, ok := c4.NewIo(client.CpFlags.Args(), uint64(1), c4_stdoutch, c4_stderrch)
		go func() {
			defer close(c4_stdoutch)
			defer close(c4_stderrch)
			if !ok {
				return
			}
			c4.CpMain(io, client.RecursiveFlag, client.VerboseFlag)
		}()

		// io.Wait()

		cp_stderr := normalize_buffer(stderr, cp_target)
		cp_stdout := normalize_buffer(stdout, cp_target)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			c4_stderr = normalize_errch(c4_stderrch, c4_target)
			wg.Done()
		}()
		go func() {
			c4_stdout = normalize_strch(c4_stdoutch, c4_target)
			wg.Done()
		}()
		wg.Wait()
		compare_slices(is, c4_stderr, cp_stderr)
		compare_slices(is, c4_stdout, cp_stdout)
		if tt.status != 0 {
			is.NotNil(cp_err)
			expected := fmt.Sprintf("exit status %d", tt.status)
			is.Equal(expected, cp_err.Error())
		}
		ok = compare_folders(is, cp_target, c4_target)
		is.OK(ok)
	}
}

// cp's output includes the name of the temp folder
// normalize_buffer replaces this specific name with a
// generic name 'targetdir' to make output comparisons easier.
func normalize_buffer(in bytes.Buffer, replace string) []string {
	out := []string{}
	for _, line := range strings.Split(in.String(), "\n") {
		if line == "" {
			continue
		}
		line = strings.Replace(line, replace, "targetdir", -1)
		out = append(out, line)
	}
	return out
}

// CpMain's error output channel includes the name of the temp folder
// normalize_errch replaces this specific name with a
// generic name 'targetdir' to make output comparisons easier.
func normalize_errch(ch chan error, replace string) []string {
	err := []string{}
	for eout := range ch {
		for _, e := range strings.Split(eout.Error(), "\n") {
			line := e

			if line == "" {
				continue
			}
			line = strings.Replace(line, replace, "targetdir", -1)
			err = append(err, line)
		}
	}
	return err
}

// CpMain's output channel includes the name of the temp folder
// normalize_strch replaces this specific name with a
// generic name 'targetdir' to make output comparisons easier.
func normalize_strch(ch chan string, replace string) []string {
	out := []string{}
	for output := range ch {
		for _, line := range strings.Split(output, "\n") {
			if line == "" {
				continue
			}
			line = strings.Replace(line, replace, "targetdir", -1)
			out = append(out, line)
		}
	}
	return out
}

// build_file_list is a utility function to convert
// glob patterns such as '*' to concrete file lists.
func build_file_list(is is.I, glob []string) []string {
	files := []string{}
	for _, g := range glob {
		f, err := filepath.Glob(g)
		is.NoErr(err)
		files = append(files, f...)
	}
	return files
}

func compare_slices(is is.I, aslice []string, bslice []string) {
	for i, a := range aslice {
		is.Equal(a, bslice[i])
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
		is.Equal(c4_map[k], v)
		// if c4_map[k] != v {

		// 	return false
		// }
	}
	return true
}
