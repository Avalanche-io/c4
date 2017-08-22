package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	flag "github.com/ogier/pflag"
)

const version_number = "0.6"

func versionString() string {
	return `c4 version ` + version_number + ` (` + runtime.GOOS + `)`
}

func main() {
	flag.Parse()
	file_list := flag.Args()
	if version_flag {
		fmt.Println(versionString())
		os.Exit(0)
	}

	if len(file_list) == 0 {
		identify_pipe()
	} else if len(file_list) == 1 && !(recursive_flag || include_meta) && depth == 0 {
		identify_file(file_list[0])
	} else {
		identify_files(file_list)
	}
	os.Exit(0)
}

func identify_pipe() {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		printID(encode(reader))
	} else {
		flag.Usage()
	}
}

func identify_file(filename string) {
	path, err := filepath.Abs(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", filename, err)
		os.Exit(1)
	}
	id := walkFilesystem(-1, path, "")
	printID(id)
}

func identify_files(file_list []string) {
	for _, file := range file_list {
		path, err := filepath.Abs(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", file, err)
			os.Exit(1)
		}
		if depth < 0 {
			depth = 0
		}
		walkFilesystem(depth, path, "")
	}
}
