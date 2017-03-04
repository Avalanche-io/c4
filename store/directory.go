package store

import "io"

type directory []string

// directoryscanner implements bufio.SplitFunc.  It tokenizes byte(0) delimited
// strings.  An error is returned if a string of length 0 is parsed (i.e. two
// byte(0)s in a row)
func directoryscanner(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == byte(0) {
			if i == 0 {
				return 0, nil, ErrDirUnderflow
			}
			return i + 1, data[:i], nil
		}
	}
	// no token returned yet
	switch {
	case len(data) == 0 && atEOF:
		return 0, nil, ErrDirUnderflow
	case len(data) != 0 && atEOF:
		return len(data), data, nil
	case len(data) == 0 && !atEOF:
		return 0, nil, io.EOF
	case len(data) != 0 && !atEOF:
		return 0, nil, nil
	}

	return 0, nil, nil
}
