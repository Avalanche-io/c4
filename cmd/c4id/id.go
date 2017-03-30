package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	c4 "github.com/avalanche-io/c4/id"
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

func fileID(path *string) (*c4.ID, error) {
	f, err := os.Open(*path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	id := c4.Identify(f)
	return id, nil
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
