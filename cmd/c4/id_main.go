package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"

	flag "github.com/ogier/pflag"

	"github.com/etcenter/c4/asset"
	"github.com/etcenter/c4/fs"
)

type ColorFunc func(...interface{}) string

var (
	bold    ColorFunc
	red     ColorFunc
	yellow  ColorFunc
	green   ColorFunc
	blue    ColorFunc
	magenta ColorFunc
	cyan    ColorFunc
	white   ColorFunc
)

func init() {
	bold = color.New(color.Bold).SprintFunc()
	red = color.New(color.FgRed).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	green = color.New(color.FgGreen).SprintFunc()
	blue = color.New(color.FgBlue).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	cyan = color.New(color.FgCyan).SprintFunc()
	white = color.New(color.FgWhite).SprintFunc()
	_ = red
	_ = yellow
	_ = green
	_ = blue
	_ = magenta
	_ = cyan
	_ = white

}

func id_main(f *flag.FlagSet) {
	file_list := f.Args()
	if len(file_list) == 0 {
		identify_pipe()
		// } else if len(file_list) == 1 && !(recursive_flag || include_meta) && depth == 0 {
		// walk_one(file_list[0])
	} else {
		walk(file_list)
	}
}

func identify_pipe() {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(os.Stdin)
		id, err := asset.Identify(reader)
		if err != nil {
			panic(err)
		}
		printID(id)
	} else {
		flag.Usage()
	}
}

// func walk_one(filename string) {
// 	fmt.Fprintf(os.Stderr, "%s\n", cyan("walk_one"))
// 	path, err := filepath.Abs(filename)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", filename, err)
// 		os.Exit(1)
// 	}
// 	// ch := make(chan attributes.FsInfo)
// 	info, err := os.Stat(path)
// 	if err != nil {
// 		panic(err)
// 	}
// 	item := attributes.NewFsInfo(info)

// 	ch := item.EncodedNestedJsonChan(os.Stdout)
// 	defer close(ch)

// 	id, err := walkFilesystem(-1, path, "", ch)
// 	if err != nil {
// 		panic(err)
// 	}
// 	printID(id)
// 	// attributes.JsonEncodeKVChan(os.Stdout, ch)
// 	// if err != nil {
// 	// 	panic(err)
// 	// }
// }

func walk(file_list []string) {
	start := time.Now()
	threads := 8

	wd, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}
	f := fs.New(wd)
	if f == nil {
		panic("Failed to create c4.FileSystem walker.")
	}
	f.IdWorkers(threads)
	ch := f.Add(file_list...)
	for n := range ch {
		fmt.Fprintf(os.Stderr, "%s\n", cyan(n.Name))
	}
	f.Wait()
	data, err := json.Marshal(f)
	if err != nil {
		panic(err)
	}
	d := time.Now().Sub(start)
	fmt.Fprintf(os.Stderr, "%s\n", cyan(d))
	fmt.Println(string(data))
	// fmt.Fprintf(os.Stderr, "%s\n", cyan("walk_all"))

	// for _, file := range file_list {
	// 	path, err := filepath.Abs(file)
	// 	if err != nil {
	// 		fmt.Fprintf(os.Stderr, "Unable to find absolute path for %s. %s\n", file, err)
	// 		os.Exit(1)
	// 	}

	// 	info, err := os.Stat(path)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	item := attributes.NewFsInfo(info)

	// 	if depth < 0 {
	// 		depth = 0
	// 	}

	// 	// info, err := os.Stat(path)
	// 	// if err != nil {
	// 	// 	panic(err)
	// 	// }
	// 	// item := attributes.NewFsInfo(info)

	// 	// ch := make(chan attributes.FsInfo)
	// 	ch := item.EncodedNestedJsonChan(os.Stdout)

	// 	// fmt.Fprintf(os.Stdout, "{")

	// 	id, err := walkFilesystem(depth, path, "", ch)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	_ = id
	// 	// fmt.Fprintf(os.Stdout, "}")

	// 	// printID(id)

	// 	// err = attributes.JsonEncodeKVChan(os.Stdout, ch)
	// 	// if err != nil {
	// 	// 	panic(err)
	// 	// }
	// }
}
