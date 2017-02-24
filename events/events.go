package events

import (
	"sync"
)

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
func merge(cs ...<-chan Event) <-chan Event {
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

// from: https://gist.github.com/icholy/5449954
type Acc string

func NewWordChan(words []Event) Chan {
	stream := make(Chan)
	go func() {
		for _, w := range words {
			stream <- w
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

func (in Chan) Reduce(fn func(Acc, Event) (Acc, Acc)) chan Acc {
	out := make(chan Acc)
	go func() {
		var val, acc Acc
		for x := range in {
			acc, val = fn(acc, x)
			if val != "" {
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

func (in Chan) ReduceAll(fn func(Acc, Event) Acc) Acc {
	var acc Acc
	for x := range in {
		acc = fn(acc, x)
	}
	return acc
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

// func main() {

// 	reverse := func(word Event) Event {
// 		s := string(word)
// 		o := make([]rune, utf8.RuneCountInString(s))
// 		i := len(o)
// 		for _, c := range s {
// 			i--
// 			o[i] = c
// 		}
// 		return Event(o)
// 	}

// 	longerThan := func(n int) func(Event) bool {
// 		return func(w Event) bool {
// 			return len(w) > n
// 		}
// 	}

// 	not := func(s string) func(Event) bool {
// 		Event := Event(s)
// 		return func(w Event) bool {
// 			return Event != w
// 		}
// 	}

// 	words := []Event{"this", "is", "pretty", "cool", "1", "2", "3"}

// 	NewWordStream(words).Map(reverse).Filter(longerThan(2)).Map(reverse).Filter(not("this")).Do(func(w Event) {
// 		println(w)
// 	})
// }

// func (m *Manager) FilteredEvents(types ...Event) <-chan Event {
// 	filter := make(map[string]bool)
// 	for _, v := range types {
// 		filter[reflect.TypeOf(v).String()] = true
// 	}
// 	fout := make(chan Event, 1)
// 	in := m.unfiltered
// 	out := m.all
// 	stop := m.stop
// 	ctl := m.control
// 	go func() {
// 		select {
// 		case ctl <- struct{}{}:
// 		case <-time.After(1 * time.Second):
// 			fmt.Printf("ReceiveFilteredEvents is unable to take control, did you forget to Start()?\n")
// 		}

// 		fmt.Printf("ReceiveFilteredEvents - running\n")
// 		for {
// 			select {
// 			case <-ctl:
// 			case <-stop:
// 				return
// 			}
// 		}
// 	}()

// 	go func() {

// 		defer func() {
// 			close(fout)
// 		}()

// 		for e := range in {
// 			select {
// 			case out <- e:
// 				fmt.Printf("Echoing to all events channel: %v\n", e)
// 			case <-stop:
// 				fmt.Printf("ReceiveFilteredEvents - Stopping\n")
// 				return
// 			}
// 			if filter[reflect.TypeOf(e).String()] {
// 				fmt.Printf("ReceiveFilteredEvents - sending filtered event\n")
// 				fout <- e
// 			}
// 		}
// 	}()
// 	return fout
// }
