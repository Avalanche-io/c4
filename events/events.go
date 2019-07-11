package events

import (
	"sync"
)

// updated, delete me

type Event interface {
	Event() string
}

type ErrorEvent interface {
	Event() string
	Error() string
}

type Chan chan Event
type ErrorChan chan ErrorEvent
type Error string
type String string

func (s String) Event() string {
	return string(s)
}

func (e Error) Event() string {
	return string(e)
}

func (e Error) Error() string {
	return string(e)
}

func (e Chan) If(v interface{}) bool {

	switch val := v.(type) {
	case error:
		e <- Error(val.Error())
		return true
	case string:
		e <- String(val)
		return true
	case Event:
		e <- val
		return true
	case nil:
		return false
	}
	return false
}

func (e ErrorChan) If(err error) bool {
	if err == nil {
		return false
	}
	e <- Error(err.Error())
	return true
}

// Keeping this here for now, but should probably moved, or removed.
func Merge(cs ...<-chan Event) <-chan Event {
	var wg sync.WaitGroup
	merged_out := make(chan Event)

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to merged_out until c is closed, then calls wg.Done.
	output := func(c <-chan Event) {
		for n := range c {
			merged_out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close merged_out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(merged_out)
	}()
	return merged_out
}

func MergeErrors(cs ...ErrorChan) ErrorChan {
	var wg sync.WaitGroup
	merged_out := make(ErrorChan)

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to merged_out until c is closed, then calls wg.Done.
	output := func(c ErrorChan) {
		for n := range c {
			merged_out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close merged_out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(merged_out)
	}()
	return merged_out
}

// from: https://gist.github.com/icholy/5449954
type Acc interface{}

func EventsChan(events []Event) Chan {
	stream := make(Chan)
	go func() {
		for _, e := range events {
			stream <- e
		}
		close(stream)
	}()
	return stream
}

func (in Chan) Map(fn func(Event) Event) Chan {
	out := make(Chan)
	go func() {
		for x := range in {
			out <- fn(x)
		}
		close(out)
	}()
	return out
}

func (in Chan) Filter(fn func(Event) bool) Chan {
	out := make(Chan)
	go func() {
		for x := range in {
			if fn(x) {
				out <- x
			}
		}
		close(out)
	}()
	return out
}

func (in Chan) Reduce(fn func(Acc, Event) (Acc, Event)) chan Event {
	out := make(chan Event)
	go func() {
		var val Event
		var acc Acc
		for x := range in {
			acc, val = fn(acc, x)
			if val != nil {
				out <- val
			}
		}
		close(out)
	}()
	return out
}

func (in Chan) Sniff(fn func(Event)) Chan {
	out := make(Chan)
	go func() {
		for x := range in {
			fn(x)
			out <- x
		}
		close(out)
	}()
	return out
}

func (in Chan) Split() (Chan, Chan) {
	out1, out2 := make(Chan), make(Chan)
	go func() {
		for x := range in {
			out1 <- x
			out2 <- x
		}
		close(out1)
		close(out2)
	}()
	return out1, out2
}

func (in Chan) ReduceAll(fn func(Acc, Event) (Acc, Event)) Event {
	var acc Acc
	var e Event
	for x := range in {
		acc, e = fn(acc, x)
	}
	return e
}

func (in Chan) Collect() []Event {
	a := make([]Event, 0, 5)
	for x := range in {
		a = append(a, x)
	}
	return a
}

func (in Chan) Do(fn func(Event)) {
	for x := range in {
		fn(x)
	}
}
