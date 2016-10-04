package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

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

func walk(file_list []string) {
	// start := time.Now()
	threads := 8
	wd, err := os.Getwd()
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
		_ = n
		// fmt.Fprintf(os.Stderr, "%s\n", cyan(n.Name))
	}
	f.Wait()

	data, err := json.Marshal(f)
	if err != nil {
		panic(err)
	}
	// d := time.Now().Sub(start)
	fmt.Println(string(data))
}
