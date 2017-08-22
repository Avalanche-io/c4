package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/etcenter/c4/asset"
	"golang.org/x/crypto/ssh/terminal"
)

func encode(src io.Reader) *asset.ID {
	e := asset.NewIDEncoder()
	_, err := io.Copy(e, src)
	if err != nil {
		panic(err)
	}
	return e.ID()
}

func fileID(path string) (id *asset.ID) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to identify %s. %v\n", path, err)
		os.Exit(1)
	}
	id = encode(f)
	f.Close()
	return
}

func nullId() *asset.ID {
	e := asset.NewIDEncoder()
	io.Copy(e, strings.NewReader(``))
	return e.ID()
}

func printID(id *asset.ID) {
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Printf("%s\n", id.String())
	} else {
		fmt.Printf("%s", id.String())
	}
}
