package main

import (
	"fmt"
	"os"

	"github.com/etcenter/c4/asset"
	"golang.org/x/crypto/ssh/terminal"
)

func printID(id *asset.ID) {
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Printf("%s\n", id.String())
	} else {
		fmt.Printf("%s", id.String())
	}
}
