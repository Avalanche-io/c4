package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	flag "github.com/ogier/pflag"
)

const (
	cp_usage string = "usage: cp [-R [-H | -L | -P]] [-fi | -n] [-apvX] source_file target_file\n       cp [-R [-H | -L | -P]] [-fi | -n] [-apvX] source_file ... target_directory\n"
)

type CpError string

func (e CpError) Error() string {
	return string(e)
}

func CpMain(f *flag.FlagSet, stdioch chan string, stderrch chan error) {
	args := f.Args()
	if len(args) == 0 {
		stderrch <- CpError(cp_usage)
		return
	}
	if len(args) == 1 && args[0] == "" {
		stderrch <- CpError(cp_usage)
		return
	}

	target_arg := args[len(args)-1]
	target, err := filepath.EvalSymlinks(target_arg)
	if err != nil {
		stderrch <- err
		return
	}
	for _, file := range args[:len(args)-1] {
		if file == "" {
			continue
		}
		info, err := os.Stat(file)
		if err != nil {
			stderrch <- err
			continue
		}
		if info.IsDir() {
			if recursive_cp_flag {
				err := filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
					if verbose_cp_flag {
						stdioch <- fmt.Sprintf("%s -> %s\n", path, target_arg+string(os.PathSeparator)+path)
					}
					return mkdirOrCopy(path, info, target)
				})
				if err != nil {
					stderrch <- err
					continue
				}
			} else {
				stderrch <- CpError(fmt.Sprintf("cp: %s is a directory (not copied).\n", file))
				continue
			}
		} else {
			if verbose_cp_flag {
				stdioch <- fmt.Sprintf("%s -> %s\n", file, target_arg+string(os.PathSeparator)+file)
			}
			err := mkdirOrCopy(file, info, target)
			if err != nil {
				stderrch <- err
				continue
			}
		}
	}
	return
}

func mkdirOrCopy(path string, info os.FileInfo, target string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	src_path := cwd + string(os.PathSeparator) + path
	target_path := target + string(os.PathSeparator) + path

	target_info, err := os.Stat(target_path)
	if err == nil && os.SameFile(info, target_info) {
		return CpError("Failed to copy files identical " + src_path)
	} else if !os.IsNotExist(err) {
		return err
	}

	if info.IsDir() {
		return os.MkdirAll(target_path, info.Mode().Perm())
	} else if !info.Mode().IsRegular() {
		return CpError("Failed to copy non regular file " + src_path)
	}

	return copyFileContents(src_path, target_path)
}

func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}

// func cp_main(f *flag.FlagSet) {
// 	file_list := f.Args()
// 	_ = file_list
// 	queue_names := []string{"Id"}
// 	for _, t := range target_cp_flag {
// 		queue_names = append(queue_names, t)
// 	}
// 	engine := fs.NewTaskEngine(queue_names)
// 	engine.Start()
// 	defer func() {
// 		engine.Close()
// 	}()

// 	engine.TaskHandler("Id", func(src string, b *fs.Buffer) {
// 		// time.Sleep(time.Duration(1) * time.Millisecond)
// 		fmt.Printf("Id task for: %s\n", src)
// 	})
// 	for _, n := range queue_names {
// 		engine.TaskHandler(n, func(src string, b *fs.Buffer) {
// 			fmt.Printf("Copy to '%s' task: %s\n", n, src)
// 		})
// 	}

// 	go func() {
// 		for _, n := range file_list {
// 			engine.Add(n)
// 		}
// 		engine.InputDone()
// 	}()
// }

// Current version:

// func cp_main(fl *flag.FlagSet) {
// 	filelist := fl.Args()

// 	// wd, err := os.Getwd()
// 	// if err != nil {
// 	// 	panic(err)
// 	// }

// 	wd, err := os.Getwd()
// 	if err != nil {
// 		panic(err)
// 	}
// 	// wd = filepath.Join(wd, filelist[0])
// 	// f := fs.New(wd)
// 	// f.Add(filelist...)

// 	targets := []string{"Id"}
// 	targets = append(targets, target_cp_flag.List...)
// 	// targets := target_cp_flag
// 	// targets.List
// 	f := fs.New(wd)
// 	var sources []string
// 	for _, path := range filelist {
// 		var source string
// 		if !filepath.IsAbs(path) {
// 			prefix, err := os.Getwd()
// 			if err != nil {
// 				panic(err)
// 			}
// 			p := filepath.Join(prefix, path)
// 			source = p[len(prefix)+1:]
// 		} else {
// 			source = path
// 		}
// 		sources = append(sources, source)

// 	}

// 	f.Add(sources...)
// 	engine := fs.NewTaskEngine(targets, 20)
// 	errCh := engine.Start()
// 	go func() {
// 		for e := range errCh {
// 			fmt.Fprintf(os.Stderr, "Error: %s\n", red(e.Error()))
// 		}
// 	}()
// 	defer func() {
// 		engine.EnqueueFS(f)
// 		engine.Close()
// 	}()

// 	folder_ids := fs.NewEmptyItem()
// 	id_counts := fs.NewEmptyItem()

// 	engine.TaskHandler("Id", func(item *fs.Item, b *fs.Buffer) error {
// 		dirs := strings.Split(item.Path(), string(os.PathSeparator))
// 		dirs = dirs[1 : len(dirs)-1]
// 		count := 0

// 		var dirpath string
// 		if !item.IsDir() {
// 			id, err := asset.Identify(bytes.NewReader(b.Bytes()))
// 			if err != nil {
// 				return err
// 			}
// 			item.SetAttribute("id", id)
// 			fmt.Printf("%s: %s\n", item.Id(), item.Path())

// 			dirpath = filepath.Dir(item.Path())
// 			if parent := folder_ids.Get(dirpath); parent == nil {
// 				if c := id_counts.Get(dirpath); c != nil {
// 					count = c.(int)
// 				}
// 				id_counts.Set(dirpath, count-1)
// 				return nil
// 			}

// 		} else {

// 			dirpath = item.Path()
// 		}

// 		if c := id_counts.Get(dirpath); c != nil {
// 			count = c.(int)
// 		} else {
// 			id_counts.Set(dirpath, 0)
// 		}

// 		if d := folder_ids.Get(dirpath); d != nil {
// 			var ids asset.IDSlice
// 			dir := d.(*fs.Item)
// 			for ele := range dir.Iterator(nil) {
// 				if ele.Key == "." {
// 					continue
// 				}
// 				if child := ele.Value; child != nil {
// 					c := (child.(*fs.Item))
// 					if id := c.GetAttribute("id"); id != nil {
// 						ids.Push(id.(*asset.ID))
// 					} else {
// 						return nil
// 					}
// 				} else {
// 					return nil
// 				}
// 			}
// 			id, err := ids.ID()
// 			if err != nil {
// 				return err
// 			}
// 			dir.SetAttribute("id", id)

// 		}
// 		return nil
// 	})

// 	for _, targetpath := range targets[1:] {
// 		root := targetpath
// 		if !filepath.IsAbs(targetpath) {
// 			root = filepath.Join(wd, targetpath)
// 		}

// 		engine.TaskHandler(targetpath, func(item *fs.Item, b *fs.Buffer) error {
// 			prefix, err := os.Getwd()
// 			if err != nil {
// 				panic(err)
// 			}

// 			target := root + item.Path()[len(prefix):]

// 			os.MkdirAll(root, 0777)

// 			if item.IsDir() {
// 				os.MkdirAll(target, 0777)
// 			} else {
// 				os.MkdirAll(filepath.Dir(target), 0777)
// 				f, err := os.Create(target)
// 				if err != nil {
// 					return err
// 				}
// 				defer f.Close()
// 				n, err := f.Write(b.Bytes())
// 				if err != nil {
// 					return err
// 				}
// 				if int64(n) != item.Size() {
// 					panic("Write failed: " + target)
// 				}
// 			}
// 			return nil
// 		})
// 	}

// 	// f := fs.New(wd)
// 	// f.Add(filelist...)
// 	// engine.EnqueueFS(f)
// }
