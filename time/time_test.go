package time_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cheekybits/is"

	c4 "github.com/avalanche-io/c4/time"
)

const testtime = "Fri Aug 29 2:14:00 EDT 1997"

func TestNewTime(t *testing.T) {
	// init
	is := is.New(t)
	loc, err := time.LoadLocation("US/Eastern")
	is.NoErr(err)
	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	is.NoErr(err)
	// ut 'universal time'
	ut := c4.NewTime(testday)
	is.Equal("1997-08-29T06:14:00Z", string(ut))
}

func equalTime(t1 time.Time, t2 time.Time) bool {
	return t1.Sub(t2) < time.Millisecond
}

func TestNow(t *testing.T) {
	// init
	is := is.New(t)
	ut := c4.Now()
	is.True(equalTime(ut.AsTime(), time.Now()))
}

func TestTimeNil(t *testing.T) {
	// init
	is := is.New(t)
	var ut c4.Time
	is.True(ut.Nil())
}

func TestTimeAsTime(t *testing.T) {
	// init
	is := is.New(t)
	loc, err := time.LoadLocation("US/Eastern")
	is.NoErr(err)
	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	is.NoErr(err)
	ut := c4.NewTime(testday)
	is.Equal(ut.AsTime(), testday.In(time.Local))
}

func TestTimeAge(t *testing.T) {
	// init
	is := is.New(t)
	loc, err := time.LoadLocation("US/Eastern")
	is.NoErr(err)
	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	is.NoErr(err)
	ut := c4.NewTime(testday)
	dif := time.Now().Sub(testday)
	// There will be slight difference in ut.Age call to time.Now, and
	// the one in the test, perhaps 10 microseconds is reasonable margin.
	// Or else use the equalTime function for much larger margin.
	is.Equal(ut.Age()/(time.Millisecond), dif/(time.Millisecond))
}

func TestTimeJSON(t *testing.T) {
	is := is.New(t)
	loc, err := time.LoadLocation("US/Eastern")
	is.NoErr(err)
	testday, err := time.ParseInLocation(time.UnixDate, testtime, loc)
	is.NoErr(err)
	ut := c4.NewTime(testday)
	is.Equal("1997-08-29T06:14:00Z", string(ut))
	data, err := json.Marshal(ut)
	is.NoErr(err)
	is.Equal("\"1997-08-29T06:14:00Z\"", string(data))
}
