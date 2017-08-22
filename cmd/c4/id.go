package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	c4 "github.com/Avalanche-io/c4/id"
	"golang.org/x/crypto/ssh/terminal"
)

func encode(src io.Reader) *c4.ID {
	e := c4.NewEncoder()
	_, err := io.Copy(e, src)
	if err != nil {
		panic(err)
	}
	return e.ID()
}

func fileID(path string) (id *c4.ID) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to identify %s. %v\n", path, err)
		os.Exit(1)
	}
	id = encode(f)
	f.Close()
	return
}

func nullId() *c4.ID {
	e := c4.NewEncoder()
	io.Copy(e, strings.NewReader(``))
	return e.ID()
}

func printID(id *c4.ID) {
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Printf("%s\n", id.String())
	} else {
		fmt.Printf("%s", id.String())
	}
}
