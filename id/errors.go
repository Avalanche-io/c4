package id

import (
	"strconv"
)

type errBadChar int

func (e errBadChar) Error() string {
	return "non c4 id character at position " + strconv.Itoa(int(e))
}

type errBadLength int

func (e errBadLength) Error() string {
	return "c4 ids must be 90 characters long, input length " + strconv.Itoa(int(e))
}

type errNil struct{}

func (e errNil) Error() string {
	return "unexpected nil id"
}

type errInvalidTree struct{}

func (e errInvalidTree) Error() string {
	return "invalid tree data"
}
