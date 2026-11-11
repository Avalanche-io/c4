package reconcile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// A link whose target is already right: a stated time that differs is set
// on the link itself (not its target), and a null time leaves the link's
// own time alone (a null never overwrites). This is the planner change
// every caller of Plan and Apply sees (c4 restore; c4sh cp).
func TestPlanLinkTimeOnExistingLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on Windows")
	}
	if !linkTimeSupported {
		t.Skip("this platform cannot set a symlink's own time")
	}
	existing := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	stated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	targetTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, c := range []struct {
		name string
		ts   time.Time
		want time.Time
	}{
		{"stated time is set", stated, stated},
		{"null time leaves the link alone", c4m.NullTimestamp(), existing},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(filepath.Join(dir, "a.txt"), targetTime, targetTime); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "link")
			if err := os.Symlink("a.txt", link); err != nil {
				t.Fatal(err)
			}
			if err := setLinkTime(link, existing); err != nil {
				t.Fatal(err)
			}

			target := c4m.NewManifest()
			id := c4.Identify(strings.NewReader("alpha"))
			target.AddEntry(&c4m.Entry{Name: "a.txt", Mode: 0o644, Size: 5, C4ID: id, Timestamp: targetTime})
			target.AddEntry(&c4m.Entry{Name: "link", Mode: os.ModeSymlink | 0o777, Size: 5, Target: "a.txt", Timestamp: c.ts})

			rec := New()
			plan, err := rec.Plan(target, dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := rec.Apply(plan, dir); err != nil {
				t.Fatal(err)
			}
			li, err := os.Lstat(link)
			if err != nil {
				t.Fatal(err)
			}
			if !li.ModTime().Truncate(time.Second).Equal(c.want) {
				t.Errorf("link's own time %v, want %v", li.ModTime().UTC(), c.want)
			}
			ti, _ := os.Stat(filepath.Join(dir, "a.txt"))
			if !ti.ModTime().Truncate(time.Second).Equal(targetTime) {
				t.Errorf("the link's target was touched: %v, want %v", ti.ModTime().UTC(), targetTime)
			}
		})
	}
}
