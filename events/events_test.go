package events_test

import (
	"errors"

	"github.com/Avalanche-io/c4/events"
	"github.com/cheekybits/is"

	"testing"
)

type test_event string

func (e test_event) Event() string {
	return string(e)
}

func TestEvents(t *testing.T) {
	is := is.New(t)
	ech := make(chan events.Event)
	go func() {
		ech <- test_event("test value")
		close(ech)
	}()

	for e := range ech {
		is.Equal(e.Event(), "test value")
	}
}

func TestIf(t *testing.T) {
	is := is.New(t)
	ech := make(events.Chan)
	tests := []struct {
		Message string
		Err     bool
		Nil     bool
		Exp     bool
	}{{
		Message: "test message",
		Err:     false,
		Nil:     false,
		Exp:     true,
	},
		{
			Message: "test message 2",
			Err:     true,
			Nil:     false,
			Exp:     true,
		},
		{
			Message: "",
			Err:     false,
			Nil:     true,
			Exp:     false,
		},
	}
	go func() {
		for _, test := range tests {
			if test.Err {
				is.Equal(ech.If(errors.New(test.Message)), test.Exp)
				continue
			}
			if test.Nil {
				is.Equal(ech.If(nil), test.Exp)
				continue
			}
			is.Equal(ech.If(test_event(test.Message)), test.Exp)
		}
		close(ech)
	}()

	i := 0
	for event := range ech {
		is.Equal(event.Event(), tests[i].Message)
		i++
	}

	// event count should equal the number of non nil tests
	is.Equal(i, 2)

}

func TestError(t *testing.T) {
	is := is.New(t)
	// err := events
	err := make(events.ErrorChan)
	tests := []struct {
		Message string
		Err     bool
		Nil     bool
		Exp     bool
	}{
		{
			Message: "test message",
			Err:     false,
			Nil:     false,
			Exp:     true,
		},
		{
			Message: "test message 2",
			Err:     true,
			Nil:     false,
			Exp:     true,
		},
		{
			Message: "",
			Err:     false,
			Nil:     true,
			Exp:     false,
		},
	}

	go func() {
		for _, test := range tests {
			if test.Err {
				is.Equal(err.If(errors.New(test.Message)), test.Exp)
				continue
			}
			if test.Nil {
				is.Equal(err.If(nil), test.Exp)
				continue
			}
			// It is compile time error to try to send an event on an error channel
			// is.Equal(err.If(test_event(test.Message)), test.Exp)

			// However we can create an error event:
			is.Equal(err.If(events.Error(test.Message)), test.Exp)
		}
		close(err)
	}()

	i := 0
	for error_event := range err {
		is.Equal(error_event.Error(), tests[i].Message)
		i++
	}

	// event count should equal the number of non nil tests
	is.Equal(i, 2)
}
