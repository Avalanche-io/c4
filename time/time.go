package time

import (
	"math"
	"time"
)

// Time is a universal time value. It is stored as a string to improve
// viability and error checking, and enable unbounded range and precision.
type Time string

// New creates a new universal time value from a time.Time value.
func NewTime(t time.Time) Time {
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

// AsTime() return the universal time value as a standard library time.Time
// value.
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
// when Age is called.
//
// If universal time is not set, Age returns the maximum possible duration
// (i.e. time.Duration(math.MaxInt64)).
func (t Time) Age() time.Duration {
	if t.Nil() {
		return time.Duration(math.MaxInt64)
	}
	tm := t.AsTime()
	return time.Now().UTC().Sub(tm)
}

func (t Time) Add(d time.Duration) Time {
	return Time(t.AsTime().Add(d).UTC().Format(time.RFC3339))
}

// Common durations, not supported buy the standard library due
// local time variances that do not occur in universal time.
const (
	Day   time.Duration = time.Hour * 24
	Week                = Day * 7
	Month               = Day * 30
	Year                = Day * 365
)
