package time_test

import (
	"encoding/json"
	"testing"
	"time"

	c4 "github.com/Avalanche-io/c4/time"
)

const testtime = "Fri Aug 29 2:14:00 EDT 1997"

func TestNewTime(t *testing.T) {
	// init
	loc, err := time.LoadLocation("US/Eastern")
	if err != nil {
		t.Errorf("error error loading location: %s", err)
	}

	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	if err != nil {
		t.Errorf("error parsing location: %s", err)
	}

	ut := c4.NewTime(testday)
	if "1997-08-29T06:14:00Z" != string(ut) {
		t.Errorf("incorrect result got %q, expected %q", string(ut), "1997-08-29T06:14:00Z")
	}
}

func equalTime(t1 time.Time, t2 time.Time) bool {
	return t1.Sub(t2) < time.Millisecond
}

func TestNow(t *testing.T) {
	// init
	ut := c4.Now()
	if !equalTime(ut.AsTime(), time.Now()) {
		t.Errorf("incorrect result, expected times to be equal")
	}
}

func TestTimeNil(t *testing.T) {
	// init
	var ut c4.Time
	if !ut.Nil() {
		t.Errorf("expected uninitialized time to be nil")
	}
}

func TestTimeAsTime(t *testing.T) {

	loc, err := time.LoadLocation("US/Eastern")
	if err != nil {
		t.Errorf("error error loading location: %s", err)
	}

	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	if err != nil {
		t.Errorf("error parsing location: %s", err)
	}

	ut := c4.NewTime(testday)
	if ut.AsTime() != testday.In(time.Local) {
		t.Errorf("incorrect result got %q, expected %q", ut.AsTime(), testday.In(time.Local))
	}
}

func TestTimeAge(t *testing.T) {

	loc, err := time.LoadLocation("US/Eastern")
	if err != nil {
		t.Errorf("error error loading location: %s", err)
	}

	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	if err != nil {
		t.Errorf("error parsing location: %s", err)
	}

	ut := c4.NewTime(testday)
	dif := time.Now().Sub(testday)
	// There will be slight difference in ut.Age call to time.Now, and
	// the one in the test, perhaps 10 microseconds is reasonable margin.
	// Or else use the equalTime function for much larger margin.
	age := ut.Age() / (time.Millisecond)
	if age != dif/(time.Millisecond) {
		t.Errorf("incorrect result got %q, expected %q", age, dif/(time.Millisecond))
	}
}

func TestTimeJSON(t *testing.T) {

	loc, err := time.LoadLocation("US/Eastern")
	if err != nil {
		t.Errorf("error error loading location: %s", err)
	}

	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	if err != nil {
		t.Errorf("error parsing location: %s", err)
	}

	ut := c4.NewTime(testday)
	if "1997-08-29T06:14:00Z" != string(ut) {
		t.Errorf("incorrect result got %q, expected %q", string(ut), "1997-08-29T06:14:00Z")
	}

	data, err := json.Marshal(ut)
	if err != nil {
		t.Errorf("error marshaling json: %s", err)
	}

	if "\"1997-08-29T06:14:00Z\"" != string(data) {
		t.Errorf("incorrect result got %q, expected %q", string(data), "\"1997-08-29T06:14:00Z\"")
	}
}

func TestTimeAdd(t *testing.T) {

	loc, err := time.LoadLocation("US/Eastern")
	if err != nil {
		t.Errorf("error error loading location: %s", err)
	}

	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	if err != nil {
		t.Errorf("error parsing location: %s", err)
	}

	ut := c4.NewTime(testday)
	if "1997-08-30T06:14:00Z" != string(ut.Add(c4.Day)) {
		t.Errorf("incorrect result got %q, expected %q", string(ut.Add(c4.Day)), "1997-08-30T06:14:00Z")
	}
}
