package store

import "errors"

type mkdirError string
type dirError string
type noIdError string

var ErrDirUnderflow error = errors.New("string of length 0 in directory list")

func (e mkdirError) Error() string {
	return "mkdir error: " + string(e)
}

func (e dirError) Error() string {
	return "directory error: " + string(e)
}

func (e noIdError) Error() string {
	return string(e) + " unexpected nil ID"
}
