package time

import (
	"fmt"
	"math"
	"time"
)

type Time string

// New creates a new universal time value from a time.Time value.
func NewTime(t time.Time) Time {
	fmt.Printf("")
	return Time(t.UTC().Format(time.RFC3339))
}

// Now creates a new universal time value set to the current time.
func Now() Time {
	return Time(time.Now().UTC().Format(time.RFC3339))
}

// Nil returns true if the universal time is the nil value.
func (t Time) Nil() bool {
	if len(t) == 0 {
		return true
	}
	return false
}

// AsTime() return the universal time value as a time.Time value.
// If the universal time value is not valid (nil, or unparseable)
// then a time.Time nil value is returned.
func (t Time) AsTime() time.Time {
	if t.Nil() {
		return time.Time{}
	}
	tm, err := time.Parse(time.RFC3339, string(t))
	if err != nil {
		return time.Time{}
	}
	return tm.In(time.Local)
}

// Age returns the time.Duration between the universal time value and
// Age is called.
//
// If universal time is not set, Age returns
// time.Duration(math.MaxInt64) (i.e. the maximum possible duration).
func (t Time) Age() time.Duration {
	if t.Nil() {
		return time.Duration(math.MaxInt64)
	}
	tm := t.AsTime()
	return time.Now().UTC().Sub(tm)
}
