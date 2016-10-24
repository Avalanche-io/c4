package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	flag "github.com/ogier/pflag"
)

const version_number = "0.7.1"

func versionString() string {
	return `c4 version ` + version_number + ` (` + runtime.GOOS + `)`
}

func main() {
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(0)
	}
	switch os.Args[1] {
	case "-h", "--help":
		flag.Usage()
		os.Exit(0)
	case "-version":
		fmt.Fprintf(os.Stderr, versionString())
		os.Exit(0)
	case "id":
		if err := id_flags.Parse(os.Args[2:]); err == nil {
			id_main(id_flags)
		} else {
			fmt.Fprintf(os.Stderr, "id_flags %s\n", err)
		}
	case "cp":
		if err := CpFlags.Parse(os.Args[2:]); err == nil {
			// fmt.Fprintf(os.Stderr, "os.Args[2:0] %v\n", os.Args[2:])
			// fmt.Fprintf(os.Stderr, "cp_flags %v\n", cp_flags)
			// os.Exit(0)

			stdoutch := make(chan string)
			stderrch := make(chan error)

			go func() {
				CpMain(CpFlags, stdoutch, stderrch)
				close(stdoutch)
				close(stderrch)
			}()

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				for err := range stderrch {
					fmt.Fprintf(os.Stderr, "%s", err.Error())
				}
				wg.Done()
			}()
			go func() {
				for str := range stdoutch {
					fmt.Fprintf(os.Stdout, "%s", str)
				}
				wg.Done()
			}()
			wg.Wait()

			// CpMain(CpFlags)
			// if err != nil {
			// 	fmt.Fprintf(os.Stderr, "%s", err.Error())
			// }
		} else {
			fmt.Fprintf(os.Stderr, "Cp_flags %s\n", err)
		}
	default:
		flag.Usage()
		os.Exit(0)
	}
}
