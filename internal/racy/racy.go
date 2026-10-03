// Package racy applies git's "racy git" rule to c4m observations.
//
// c4m timestamps have one-second precision, and downstream consumers
// (reconcile's trusted-metadata planning, guided rescans) trust a matching
// (size, mtime-second) pair to skip hashing. A file whose mtime falls in or
// after the second its scan began could still be rewritten, with the same
// size, inside that same mtime second after it was read. Recording that
// observation would let a later trust check keep stale bytes.
//
// So an observation is only recorded once the wall clock has moved past
// the file's mtime second: a racy file is waited out and re-observed, for
// a bounded number of rounds, and a file still changing after that is
// recorded with a null timestamp, which honestly says "not a stable
// observation" and disables metadata trust downstream.
package racy

import "time"

// Rounds bounds how many times a racy file is waited out and re-observed.
const Rounds = 3

// Horizon bounds how far past the current second an mtime may lie and
// still be waited out. Filesystems with coarse timestamps (FAT rounds to
// two seconds) or a slightly fast server clock can stamp a just-written
// file a little in the future; an mtime further out than this was set
// explicitly, not by a write during the scan, and waiting for it would be
// unbounded.
const Horizon = 2 * time.Second

// Test seams. Now and Sleep form the wall clock; Recheck, when non-nil,
// runs after racy files are detected and before each re-observation round
// with the round number (1-based) and the number of files pending.
var (
	Now     = time.Now
	Sleep   = time.Sleep
	Recheck func(round, pending int)
)

// Second truncates t to its whole wall-clock second.
func Second(t time.Time) time.Time {
	return t.Truncate(time.Second)
}

// Start returns the second a scan begins in.
func Start() time.Time {
	return Second(Now())
}

// Is reports whether mtime is racy for a scan that began in second start:
// it falls in that second or later, and is no further than Horizon past
// the current second (see Horizon).
func Is(mtime, start time.Time) bool {
	m := Second(mtime)
	return !m.Before(start) && !m.After(Second(Now()).Add(Horizon))
}

// Settle waits out and re-observes the racy files numbered 0..n-1.
//
// mtime(i) returns file i's most recently observed mtime. observe(i)
// re-reads file i (stat, read, stat), records the result, and returns the
// new mtime and whether the read was consistent (both stats agreed and the
// file is still a regular file). Each round waits until the wall clock is
// past every pending mtime second, then re-observes; a file whose new
// mtime is still racy against that round's start, or whose read was
// inconsistent, goes another round.
//
// Settle returns the files still unsettled after Rounds rounds. The caller
// records their timestamps as null.
func Settle(n int, mtime func(i int) time.Time, observe func(i int) (time.Time, bool)) []int {
	pending := make([]int, n)
	for i := range pending {
		pending[i] = i
	}
	for round := 1; round <= Rounds && len(pending) > 0; round++ {
		if Recheck != nil {
			Recheck(round, len(pending))
		}

		// Wait until the clock is strictly past the latest pending mtime
		// second. An mtime beyond Horizon cannot be waited out; it stays
		// pending and ends unsettled.
		var latest time.Time
		limit := Second(Now()).Add(Horizon)
		for _, i := range pending {
			m := Second(mtime(i))
			if m.After(limit) {
				continue
			}
			if m.After(latest) {
				latest = m
			}
		}
		target := latest.Add(time.Second)
		for now := Now(); now.Before(target); now = Now() {
			Sleep(target.Sub(now))
		}

		start := Start()
		var next []int
		for _, i := range pending {
			m, ok := observe(i)
			if !ok || !Second(m).Before(start) {
				next = append(next, i)
			}
		}
		pending = next
	}
	return pending
}
