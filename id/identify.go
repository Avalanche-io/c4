package id

import (
	"io"
)

// Generate an id from an io.Reader
func Identify(src io.Reader) *ID {
	e := NewEncoder()
	_, err := io.Copy(e, src)
	if err != nil && err != io.EOF {
		return nil
	}
	return e.ID()
}
