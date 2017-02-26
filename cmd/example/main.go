package main

import (
	"fmt"
	"io"
	"os"

	c4 "github.com/Avalanche-io/c4/id"
)

func main() {
	file := "main.go"
	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// create a ID encoder.
	e := c4.NewIDEncoder()
	// the encoder is an io.Writer
	_, err = io.Copy(e, f)
	if err != nil {
		panic(err)
	}
	// ID will return a *c4.ID.
	// Be sure to be done writing bytes before calling ID()
	id := e.ID()
	// use the *c4.ID String method to get the c4id string
	fmt.Printf("C4id of \"%s\": %s\n", file, id)
	return
}
