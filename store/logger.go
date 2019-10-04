package store

import (
	"fmt"
	"io"

	"github.com/Avalanche-io/c4"
)

var _ Store = &Logger{}

type LoggerFlags uint32

const (
	LogOpen LoggerFlags = 1 << iota
	LogCreate
	LogRemove
	LogRead
	LogWrite
	LogClose
	LogError
	LogInvalidID
	LogEof
)

// A Logger store wraps a c4 Store and logs a cusomizable set of Store function.
type Logger struct {

	// the wrapped Store to be logged
	s Store

	// Writer for log output
	logout io.Writer

	// flaggs for what logs to output
	flags LoggerFlags
}

// NewLogger creates a new Logger Store that wrapps a Store and logs function
// calls and errors to the Store and the io.ReadCloser and io.WriteCloser the
// Logger Store produces. Which function calls and errors are logged is
// controlled by setting the appropreate flags. If flags is set to 0 then all
// flags are enabled so all functions and errors are logged.
func NewLogger(s Store, log io.Writer, flags LoggerFlags) *Logger {
	if flags == 0 {
		flags = LogOpen | LogCreate | LogRead | LogWrite | LogClose | LogError | LogInvalidID | LogEof
	}
	return &Logger{s, log, flags}
}

// Open logs and calls the Open method of the contained Store.
func (l *Logger) Open(id c4.ID) (io.ReadCloser, error) {
	fname := "Open"
	idstr := id.String()
	// only log if logging is enabled for this method.
	if l.flags&LogOpen != 0 {
		fmt.Fprintf(l.logout, "%s %s\n", idstr, fname)
	}

	// call the wrapped Stor's Open
	r, err := l.s.Open(id)
	if err != nil {
		// only log the error if error logging is enabled.
		if l.flags&LogError != 0 {
			fmt.Fprintf(l.logout, "%s %s error %s\n", idstr, fname, err)
		}
		return nil, err
	}

	// wrap the io.ReadCloser in a loggingReader
	return &loggingReader{r, l.logout, idstr, l.flags}, nil
}

// Create logs and calls the Create method of the contained Store.
func (l *Logger) Create(id c4.ID) (io.WriteCloser, error) {
	fname := "Create"
	idstr := id.String()
	// only log if logging is enabled for this method.
	if l.flags&LogCreate != 0 {
		fmt.Fprintf(l.logout, "%s %s\n", idstr, fname)
	}

	// call the wrapped Stor's Create
	w, err := l.s.Create(id)
	if err != nil {
		// only log the error if error logging is enabled.
		if l.flags&LogError != 0 {
			fmt.Fprintf(l.logout, "%s %s error %s\n", idstr, fname, err)
		}
		return nil, err
	}

	// wrap the io.WriteCloser in a loggingWriter
	return &loggingWriter{w, l.logout, idstr, l.flags}, nil
}

// Remove logs and calls the Remove method of the contained Store.
func (l *Logger) Remove(id c4.ID) error {
	fname := "Remove"
	idstr := id.String()
	// only log if logging is enabled for this method.
	if l.flags&LogRemove != 0 {
		fmt.Fprintf(l.logout, "%s %s\n", idstr, fname)
	}

	// call the wrapped Store's Remove
	err := l.s.Remove(id)
	if err != nil {
		// only log the error if error logging is enabled.
		if l.flags&LogError != 0 {
			fmt.Fprintf(l.logout, "%s %s error %s\n", idstr, fname, err)
		}
		return err
	}

	return nil
}

// loggingReader implements the io.ReadCloser interface wrapping an underlying
// io.ReadCloser with logging.
type loggingReader struct {

	// the wrapped io.ReadCloser
	r io.ReadCloser

	// Writer for log output
	logout io.Writer

	// id to prefix to log messages
	idstr string

	// flags for what logs to output
	flags LoggerFlags
}

// Read log and reads from the contained io.ReadCloser.
func (l *loggingReader) Read(b []byte) (int, error) {
	fname := "Read"
	// call the wrapped io.ReadCloser's Read method
	n, err := l.r.Read(b)
	// only log if logging is enabled for this method.
	if l.flags&LogRead != 0 {
		fmt.Fprintf(l.logout, "%s %s %d\n", l.idstr, fname, n)
	}
	if err == nil {
		return n, nil
	}

	switch err {
	case io.EOF:
		if l.flags&LogEof == 0 {
			return n, err
		}
	case ErrInvalidID:
		if l.flags&LogInvalidID == 0 {
			return n, err
		}
	default:
		if l.flags&LogError == 0 {
			return n, err
		}
	}
	fmt.Fprintf(l.logout, "%s %s error %s\n", l.idstr, fname, err)
	return n, err
}

// Close closes the underlying Store reader, and logs the call and any errors
// as selected by flags.
func (l *loggingReader) Close() error {
	fname := "Close"

	// only log if logging is enabled for this method.
	if l.flags&LogClose != 0 {
		fmt.Fprintf(l.logout, "%s %s\n", l.idstr, fname)
	}

	// call the wrapped io.ReadCloser's Close method
	err := l.r.Close()
	if err == nil {
		return nil
	}

	if err == ErrInvalidID {
		// return without logging if logging of ErrInvalidID is not flagged
		if l.flags&LogInvalidID == 0 {
			return err
		}
	} else if l.flags&LogError == 0 {
		return err
	}

	// otherwise log the error before returning
	fmt.Fprintf(l.logout, "%s %s error %s\n", l.idstr, fname, err)
	return err
}

// loggingWriter implements the io.WriteCloser interface wrapping an underlying
// io.WriteCloser with logging.
type loggingWriter struct {
	// the wrapped io.WriteCloser
	r io.WriteCloser

	// Writer for log output
	logout io.Writer

	// id to prefix to log messages
	idstr string

	// flags for what logs to output
	flags LoggerFlags
}

// Write writes and logs to the contianed io.WriteCloser.
func (l *loggingWriter) Write(b []byte) (int, error) {
	fname := "Write"

	// call the wrapped io.WriteCloser's Write method
	n, err := l.r.Write(b)
	// only log if logging is enabled for this method.
	if l.flags&LogWrite != 0 {
		fmt.Fprintf(l.logout, "%s %s %d\n", l.idstr, fname, n)
	}
	if err == nil {
		return n, nil
	}

	switch err {
	case io.EOF:
		if l.flags&LogEof == 0 {
			return n, err
		}
	case ErrInvalidID:
		if l.flags&LogInvalidID == 0 {
			return n, err
		}
	default:
		if l.flags&LogError == 0 {
			return n, err
		}
	}
	fmt.Fprintf(l.logout, "%s %s error %s\n", l.idstr, fname, err)
	return n, err
}

// Close closes and logs to the contianed io.WriteCloser.
func (l *loggingWriter) Close() error {
	fname := "Close"

	// only log if logging is enabled for this method.
	if l.flags&LogClose != 0 {
		fmt.Fprintf(l.logout, "%s %s\n", l.idstr, fname)
	}

	// call the wrapped io.ReadCloser's Close method
	err := l.r.Close()
	if err == nil {
		return nil
	}

	if err == ErrInvalidID {
		// return without logging if logging of ErrInvalidID is not flagged
		if l.flags&LogInvalidID == 0 {
			return err
		}
	} else if l.flags&LogError == 0 {
		return err
	}

	// otherwise log the error before returning
	fmt.Fprintf(l.logout, "%s %s error %s\n", l.idstr, fname, err)
	return err
}
