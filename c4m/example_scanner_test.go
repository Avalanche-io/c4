// +build ignore

// Run this example with: go run example_scanner_test.go /path/to/scan
package main

import (
	"fmt"
	"os"
	"runtime"
	
	"github.com/Avalanche-io/c4/c4m"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <directory>\n", os.Args[0])
		os.Exit(1)
	}
	
	path := os.Args[1]
	
	fmt.Println("Starting progressive scan of:", path)
	fmt.Println()
	fmt.Println("Controls:")
	fmt.Println("  Ctrl+C : Stop and output results")
	
	if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" {
		fmt.Println("  Ctrl+T : Show status (continue scanning)")
	} else {
		fmt.Printf("  kill -USR1 %d : Show status\n", os.Getpid())
	}
	fmt.Println()
	
	cli := c4m.NewProgressiveCLI(path,
		c4m.WithVerbose(false),
		c4m.WithProgress(true),
	)
	
	if err := cli.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}