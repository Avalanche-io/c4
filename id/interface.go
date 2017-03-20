package id

import "io"

type Identifiable interface {
	ID() *ID
}

// WriteIdentifier is an interface that includes io.Writer and is Identifiable.
// This is useful for creating interfaces that compute c4 IDs on the fly when
// writing to an io.Writer.
type WriteIdentifier interface {
	io.Writer
	Identifiable
}

// WriteCloseIdentifier interface adds io.Closer to a WriteIdentifier.
type WriteCloseIdentifier interface {
	io.WriteCloser
	Identifiable
}

type ReadCloseIdentifier interface {
	io.ReadCloser
	Identifiable
}
