package id

import "io"

// Identifiable is an interface that requires an ID() method that returns
// the c4 ID of the of the object.
type Identifiable interface {
	ID() *ID
}

// Reader is an interface that matches io.Reader and adds Identifiable.
type Reader interface {
	io.Reader
	Identifiable
}

// Writer is an interface that matches io.Writer and adds Identifiable.
type Writer interface {
	io.Writer
	Identifiable
}

// WriteCloser is an interface that matches io.WriteCloser and adds Identifiable.
type WriteCloser interface {
	io.WriteCloser
	Identifiable
}

// ReadCloser is an interface that matches io.ReadCloser and adds Identifiable.
type ReadCloser interface {
	io.ReadCloser
	Identifiable
}
