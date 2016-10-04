package main

import (
	"fmt"

	flag "github.com/ogier/pflag"
	// "os"

	"github.com/etcenter/c4/fs"
)

func cp_main(f *flag.FlagSet) {
	file_list := f.Args()
	_ = file_list
	queue_names := []string{"Id"}
	for _, t := range target_cp_flag {
		queue_names = append(queue_names, t)
	}
	engine := fs.NewTaskEngine(queue_names)
	engine.Start()
	defer func() {
		engine.Close()
	}()

	engine.TaskHandler("Id", func(src string, b *fs.Buffer) {
		// time.Sleep(time.Duration(1) * time.Millisecond)
		fmt.Printf("Id task for: %s\n", src)
	})
	for _, n := range queue_names {
		engine.TaskHandler(n, func(src string, b *fs.Buffer) {
			fmt.Printf("Copy to '%s' task: %s\n", n, src)
		})
	}

	go func() {
		for _, n := range file_list {
			engine.Add(n)
		}
		engine.InputDone()
	}()
}
