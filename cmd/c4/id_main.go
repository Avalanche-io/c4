package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	filelist := f.Args()
	if len(filelist) == 0 {
		identify_pipe()
	} else {
		walk(filelist)
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

func walk(filelist []string) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	wd = filepath.Join(wd, filelist[0])
	f := fs.New(wd)
	f.Add(filelist...)
	engine := fs.NewTaskEngine([]string{"Id"}, 20)
	errCh := engine.Start()
	go func() {
		for e := range errCh {
			fmt.Fprintf(os.Stderr, "Error: %s\n", red(e.Error()))
		}
	}()
	defer func() {
		engine.EnqueueFS(f)
		engine.Close()
	}()

	basedirs := strings.Split(wd, string(os.PathSeparator))
	folder_ids := fs.NewEmptyItem()
	id_counts := fs.NewEmptyItem()
	// pending_files := fs.NewEmptyItem()
	engine.TaskHandler("Id", func(item *fs.Item, b *fs.Buffer) error {
		dirs := strings.Split(item.Path(), string(os.PathSeparator))
		dirs = dirs[1 : len(dirs)-1]
		count := 0

		var dirpath string
		if !item.IsDir() {
			id, err := asset.Identify(b.Reader())
			if err != nil {
				return err
			}
			item.SetAttribute("id", id)
			fmt.Printf("%[1]*[2]s%s:\n", len(dirs)-len(basedirs), "", item.Name())
			fmt.Printf("  ID:%s\n", item.Id())

			dirpath = filepath.Dir(item.Path())
			if parent := folder_ids.Get(dirpath); parent == nil {
				if c := id_counts.Get(dirpath); c != nil {
					count = c.(int)
				}
				id_counts.Set(dirpath, count-1)
				return nil
			}

		} else {

			dirpath = item.Path()
		}

		if c := id_counts.Get(dirpath); c != nil {
			count = c.(int)
		} else {
			id_counts.Set(dirpath, 0)
		}

		if d := folder_ids.Get(dirpath); d != nil {
			var ids asset.IDSlice
			dir := d.(*fs.Item)
			for ele := range dir.Iterator(nil) {
				if ele.Key == "." {
					continue
				}
				if child := ele.Value; child != nil {
					c := (child.(*fs.Item))
					if id := c.GetAttribute("id"); id != nil {
						ids.Push(id.(*asset.ID))
					} else {
						return nil
					}
				} else {
					return nil
				}
			}
			id, err := ids.ID()
			if err != nil {
				return err
			}
			dir.SetAttribute("id", id)

		}
		fmt.Printf("%s:\n", item.Path())
		fmt.Printf("  name:%s\n", item.Name())
		fmt.Printf("  ID:%s\n", item.Id())
		return nil
	})

}
