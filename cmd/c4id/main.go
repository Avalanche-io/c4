package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/etcenter/c4go"
)

func main() {
	var (
		src        = flag.String("src", "", "source file (rather than stdin)")
		nolinefeed = flag.Bool("n", false, "omit final line-feed")
	)
	flag.Parse()
	var r io.Reader
	if len(*src) > 0 {
		file, err := os.Open(*src)
		if err != nil {
			log.Fatalln(err)
		}
		defer file.Close()
		r = file
	} else {
		r = os.Stdin
	}
	enc := c4.NewIDEncoder()
	if _, err := io.Copy(enc, r); err != nil {
		log.Fatalln(err)
	}
	fmt.Print(enc.ID().String())
	if !*nolinefeed {
		fmt.Println()
	}
}
