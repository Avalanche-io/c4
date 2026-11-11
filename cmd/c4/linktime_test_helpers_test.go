package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// setLinkTimeForTest dates a symlink's own time in a test tree with
// touch -h, which sets the link itself on macOS and Linux alike.
func setLinkTimeForTest(path string, t time.Time) error {
	cmd := exec.Command("touch", "-h", "-t", t.UTC().Format("200601021504.05"), path)
	cmd.Env = append(os.Environ(), "TZ=UTC")
	return cmd.Run()
}

// Where a platform cannot set a link's time, the verification target
// takes that link's time from the as-built tree and nothing else changes.
func TestLinkTimesAsBuilt(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	created := ts.Add(time.Hour)
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "a.txt", Mode: 0o644, Size: 1, Timestamp: ts})
	target.AddEntry(&c4m.Entry{Name: "link", Mode: os.ModeSymlink | 0o777, Target: "a.txt", Size: 5, Timestamp: ts})
	built := target.Copy()
	c4m.EntryPaths(built.Entries)["link"].Timestamp = created

	if got := linkTimesAsBuilt(target, built, "/dest", nil); got != target {
		t.Fatal("with nothing unset the target is returned unchanged")
	}
	got := linkTimesAsBuilt(target, built, "/dest", []string{"/dest/link"})
	if !c4m.EntryPaths(got.Entries)["link"].Timestamp.Equal(created) {
		t.Fatal("the unset link must be verified at its as-built time")
	}
	if !c4m.EntryPaths(got.Entries)["a.txt"].Timestamp.Equal(ts) {
		t.Fatal("other entries are verified as stated")
	}
	if !c4m.EntryPaths(target.Entries)["link"].Timestamp.Equal(ts) {
		t.Fatal("the target itself must not be modified")
	}
	if strings.Contains(got.Canonical(), created.Format("2006-01-02T15:04:05Z")) == false {
		t.Fatal("the verification target carries the as-built link time")
	}
}
