package c4m

import (
	"strings"
	"testing"
	"time"
)

// A zone abbreviation in a pretty timestamp names an offset only if this
// process knows it. Go's time.Parse otherwise fabricates a zone with
// offset zero and reports success, so "Jan  2 03:04:05 2026 CST" read on
// a machine outside Central time came back as 03:04:05 UTC, five or six
// hours wrong, silently. That is refused now; UTC, GMT, Z, the local
// zone's own abbreviation and numeric offsets all still parse.
func TestPrettyTimestampUnknownZoneAbbreviationIsRefused(t *testing.T) {
	defer func(l *time.Location) { time.Local = l }(time.Local)
	time.Local = time.UTC

	for _, s := range []string{"Jan  2 03:04:05 2026 UTC", "Jan 2 03:04:05 2026 GMT", "Mon Jan  2 03:04:05 UTC 2026"} {
		got, err := parseTimestamp(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC); !got.Equal(want) {
			t.Fatalf("%q parsed as %v, want %v", s, got, want)
		}
	}
	got, err := parseTimestamp("Jan  2 03:04:05 2026 -0600")
	if err != nil || !got.Equal(time.Date(2026, 1, 2, 9, 4, 5, 0, time.UTC)) {
		t.Fatalf("numeric offset: got %v, %v", got, err)
	}
	for _, s := range []string{"Jan  2 03:04:05 2026 CST", "Jan  2 03:04:05 2026 XYZ", "Mon Jan  2 03:04:05 PDT 2026"} {
		if _, err := parseTimestamp(s); err == nil || !strings.Contains(err.Error(), "unknown time zone abbreviation") {
			t.Fatalf("%q must be refused with the abbreviation named, got %v", s, err)
		}
	}

	// The local zone's own abbreviation is known and parses in local time.
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("no zoneinfo")
	}
	time.Local = chicago
	got, err = parseTimestamp("Jan  2 03:04:05 2026 CST")
	if err != nil || !got.Equal(time.Date(2026, 1, 2, 9, 4, 5, 0, time.UTC)) {
		t.Fatalf("local abbreviation: got %v, %v", got, err)
	}
}
