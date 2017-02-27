package events_test

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"github.com/cheekybits/is"

	"github.com/Avalanche-io/c4/events"
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
		Type    string
		Exp     bool
	}{
		{
			Message: "test string message",
			Type:    "String",
			Exp:     true,
		},
		{
			Message: "test error message 2",
			Type:    "Error",
			Exp:     true,
		},
		{
			Message: "test event message 3",
			Type:    "Event",
			Exp:     true,
		},
		{
			Message: "",
			Type:    "Nil",
			Exp:     false,
		},
		{
			Message: "",
			Type:    "Unknown",
			Exp:     false,
		},
	}
	go func() {
		var result bool
		for _, test := range tests {
			switch test.Type {
			case "String":
				result = ech.If(test.Message)
			case "Error":
				result = ech.If(errors.New(test.Message))
			case "Event":
				result = ech.If(test_event(test.Message))
			case "Nil":
				result = ech.If(nil)
			case "Unknown":
				result = ech.If(true) // false
			}
			is.Equal(result, test.Exp)
		}
		close(ech)
	}()

	i := 0
	for event := range ech {
		is.Equal(event.Event(), tests[i].Message)
		i++
	}

	// event count should equal the number of non nil tests
	is.Equal(i, 3)

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

func TestMergeEventsChan(t *testing.T) {
	is := is.New(t)

	ch1 := make(events.Chan)

	go func() {
		for i := 0; i < 10; i++ {
			ch1 <- events.String("String event " + strconv.Itoa(i))
		}
		close(ch1)
	}()
	eventslice := make([]events.Event, 0, 10)
	for i := 0; i < 10; i++ {
		eventslice = append(eventslice, events.String("String event "+strconv.Itoa(i)))
	}
	ch2 := events.EventsChan(eventslice)
	chout := events.Merge(ch1, ch2)

	i := 0
	for e := range chout {
		matched, err := regexp.MatchString("String event [0-9]{1,3}", e.Event())
		is.True(matched)
		is.NoErr(err)
		i++
	}
	is.Equal(i, 20)
}

func TestMap(t *testing.T) {
	is := is.New(t)
	eventslice := make([]events.Event, 0, 10)
	for i := 0; i < 10; i++ {
		eventslice = append(eventslice, events.String("String event "+strconv.Itoa(i)))
	}
	ch3 := events.EventsChan(eventslice)
	ch4 := ch3.Map(func(e events.Event) events.Event {
		if e.Event() == "String event 7" {
			return events.String("Yes")
		}
		return events.String("No")
	})
	cnt_yes := 0
	cnt_no := 0
	for e := range ch4 {
		switch e.Event() {
		case "Yes":
			cnt_yes++
		case "No":
			cnt_no++
		}
	}
	is.Equal(cnt_yes, 1)
	is.Equal(cnt_no, 9)
}

func Setup(t *testing.T) (is.I, events.Chan) {
	is := is.New(t)
	eventslice := make([]events.Event, 0, 10)
	for i := 0; i < 10; i++ {
		eventslice = append(eventslice, events.String("String event "+strconv.Itoa(i)))
	}
	ch := events.EventsChan(eventslice)
	return is, ch
}

func TestFilter(t *testing.T) {
	is, ch := Setup(t)
	chout := ch.Filter(func(e events.Event) bool {
		if e.Event() == "String event 7" {
			return true
		}
		return false
	})
	e := <-chout
	is.Equal(e.Event(), "String event 7")
}

type Range struct {
	Start int
	End   int
}

func TestReduce(t *testing.T) {
	is, ch := Setup(t)
	re := regexp.MustCompile("[^0-9]([0-9]+)")
	chout := ch.Reduce(func(st events.Acc, e events.Event) (events.Acc, events.Event) {
		state := Range{-1, -1}
		if st != nil {
			state = st.(Range)
		}
		num := re.Find([]byte(e.Event()))
		i, err := strconv.Atoi(string(num[1]))
		is.NoErr(err)
		if state.Start == -1 || state.Start > i {
			state.Start = i
		}
		if state.End == -1 || state.End < i {
			state.End = i
		}
		if state.End-state.Start == 9 {
			return nil, events.String(fmt.Sprintf("String events %d-%d", state.Start, state.End))
		}
		return state, nil
	})
	for e := range chout {
		is.Equal(e.Event(), "String events 0-9")
	}
}

func TestSniff(t *testing.T) {
	is, ch := Setup(t)
	i := 0
	cout := ch.Sniff(func(e events.Event) {
		is.Equal(e.Event(), "String event "+strconv.Itoa(i))
		i++
	})

	j := 0
	for e := range cout {
		is.Equal(e.Event(), "String event "+strconv.Itoa(j))
		j++
	}
	is.Equal(i, j)
	is.Equal(i, 10)
}

func TestSplit(t *testing.T) {
	is, ch := Setup(t)
	c1, c2 := ch.Split()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		i := 0
		for e := range c1 {
			is.Equal(e.Event(), "String event "+strconv.Itoa(i))
			i++
		}
		is.Equal(i, 10)
	}()

	go func() {
		defer wg.Done()
		i := 0
		for e := range c2 {
			is.Equal(e.Event(), "String event "+strconv.Itoa(i))
			i++
		}
		is.Equal(i, 10)
	}()
	wg.Wait()
}

func TestReduceAll(t *testing.T) {
	is, ch := Setup(t)
	re := regexp.MustCompile("[^0-9]([0-9]+)")
	e := ch.ReduceAll(func(val events.Acc, e events.Event) (events.Acc, events.Event) {
		sum := 0
		if val != nil {
			sum = val.(int)
		}
		num := re.Find([]byte(e.Event()))
		i, err := strconv.Atoi(string(num[1]))
		is.NoErr(err)
		sum += i
		return sum, events.String("Total Value " + strconv.Itoa(sum))
	})
	is.Equal(e.Event(), "Total Value 45")
}

func TestCollect(t *testing.T) {
	is, ch := Setup(t)
	eventslice := ch.Collect()
	is.Equal(len(eventslice), 10)
	for i := 0; i < 10; i++ {
		is.Equal(eventslice[i].Event(), "String event "+strconv.Itoa(i))
	}
}

func TestDo(t *testing.T) {
	is, ch := Setup(t)
	i := 0
	ch.Do(func(e events.Event) {
		is.Equal(e.Event(), "String event "+strconv.Itoa(i))
		i++
	})
	is.Equal(i, 10)
}
