package main

import (
	"bytes"
	"strings"
	"testing"
)

const mod = "example.com/m"

// events writes a go test -json stream: a package that passes with one
// test passing and one skipping, with the reason the skip gave.
func events(extra ...string) string {
	lines := []string{
		`{"Action":"run","Package":"example.com/m/a","Test":"TestRuns"}`,
		`{"Action":"pass","Package":"example.com/m/a","Test":"TestRuns"}`,
		`{"Action":"run","Package":"example.com/m/a","Test":"TestNeedsRoot"}`,
		`{"Action":"output","Package":"example.com/m/a","Test":"TestNeedsRoot","Output":"=== RUN   TestNeedsRoot\n"}`,
		`{"Action":"output","Package":"example.com/m/a","Test":"TestNeedsRoot","Output":"    a_test.go:12: needs root\n"}`,
		`{"Action":"output","Package":"example.com/m/a","Test":"TestNeedsRoot","Output":"--- SKIP: TestNeedsRoot (0.00s)\n"}`,
		`{"Action":"skip","Package":"example.com/m/a","Test":"TestNeedsRoot"}`,
		`{"Action":"output","Package":"example.com/m/a","Output":"ok  \texample.com/m/a\t0.01s\n"}`,
		`{"Action":"pass","Package":"example.com/m/a"}`,
	}
	return strings.Join(append(lines, extra...), "\n") + "\n"
}

func TestAnUnlistedSkipFails(t *testing.T) {
	var out bytes.Buffer
	if code := run(strings.NewReader(events()), &out, map[string]bool{}, mod); code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "NOT ALLOWED a TestNeedsRoot: needs root") {
		t.Fatalf("the report:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "ok  \texample.com/m/a") {
		t.Fatalf("the package line is not printed:\n%s", out.String())
	}
}

func TestAListedSkipPasses(t *testing.T) {
	var out bytes.Buffer
	allowed := map[string]bool{"a TestNeedsRoot": true, "a TestRuns": true, "b TestElsewhere": true}
	if code := run(strings.NewReader(events()), &out, allowed, mod); code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "1 tests skipped, 1 of them allowed") || !strings.Contains(out.String(), "allowed, and it did not skip: a TestRuns") || strings.Contains(out.String(), "b TestElsewhere") {
		t.Fatalf("the report:\n%s", out.String())
	}
}

func TestAFailureFailsAndShowsItsOutput(t *testing.T) {
	var out bytes.Buffer
	allowed := map[string]bool{"a TestNeedsRoot": true}
	fail := []string{
		`{"Action":"run","Package":"example.com/m/b","Test":"TestBreaks"}`,
		`{"Action":"output","Package":"example.com/m/b","Test":"TestBreaks","Output":"    b_test.go:7: got 2, want 3\n"}`,
		`{"Action":"fail","Package":"example.com/m/b","Test":"TestBreaks"}`,
		`{"Action":"fail","Package":"example.com/m/b"}`,
	}
	if code := run(strings.NewReader(events(fail...)), &out, allowed, mod); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(out.String(), "got 2, want 3") {
		t.Fatalf("the failure's output is not shown:\n%s", out.String())
	}
	out.Reset()
	build := []string{
		`{"Action":"build-output","ImportPath":"example.com/m/c","Output":"c/c.go:3:1: syntax error\n"}`,
		`{"Action":"build-fail","ImportPath":"example.com/m/c"}`,
	}
	if code := run(strings.NewReader(events(build...)), &out, allowed, mod); code != 1 {
		t.Fatalf("a build failure: exit %d, want 1", code)
	}
}

func TestTheAllowListIsPerGOOS(t *testing.T) {
	list := "# reasons after #\nlinux internal/x TestA # needs root\n* internal/y TestB\ndarwin internal/z TestC\n"
	got, err := readAllow(strings.NewReader(list), "linux", "go1.27.4")
	if err != nil {
		t.Fatal(err)
	}
	if !got["internal/x TestA"] || !got["internal/y TestB"] || got["internal/z TestC"] {
		t.Fatalf("linux allows %v", got)
	}
	if _, err := readAllow(strings.NewReader("linux internal/x\n"), "linux", "go1.27.4"); err == nil {
		t.Fatal("a line without a test was taken")
	}
}

func TestAPackageThatNeverEndsFails(t *testing.T) {
	var out bytes.Buffer
	cut := `{"Action":"start","Package":"example.com/m/c"}` + "\n" + `{"Action":"run","Package":"example.com/m/c","Test":"TestHangs"}`
	if code := run(strings.NewReader(events(cut)), &out, map[string]bool{"a TestNeedsRoot": true}, mod); code != 1 || !strings.Contains(out.String(), "c started and never ended") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
}

func TestASkipWithoutAMessageHasNoReason(t *testing.T) {
	if got := reason([]string{"=== RUN   TestX\n", "--- SKIP: TestX (0.00s)\n"}); got != "" {
		t.Fatalf("reason %q", got)
	}
}

func TestNoEventsIsNotAPass(t *testing.T) {
	var out bytes.Buffer
	if code := run(strings.NewReader(""), &out, map[string]bool{}, mod); code != 1 {
		t.Fatalf("an empty stream: exit %d, want 1:\n%s", code, out.String())
	}
}

func TestAReasonOnSeveralLinesIsKeptWhole(t *testing.T) {
	got := reason([]string{
		"=== RUN   TestX\n",
		"    x_test.go:9: needs a machine\n",
		"        set aside for it\n",
		"--- SKIP: TestX (0.00s)\n",
	})
	if got != "needs a machine set aside for it" {
		t.Fatalf("reason %q", got)
	}
}

// A Go-version qualifier allows a skip only on that Go release: the
// printable-table check may skip on Go 1.16, where the runtime's Unicode is
// older, and must not skip on current Go, where it runs.
func TestTheAllowListCanNameAGoRelease(t *testing.T) {
	list := "linux go1.16 a TestNeedsRoot # old Unicode\n* b TestOptIn # opt-in\n"

	old, err := readAllow(strings.NewReader(list), "linux", "go1.16.15")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run(strings.NewReader(events()), &out, old, mod); code != 0 {
		t.Errorf("on go1.16 the qualified skip is allowed, got exit %d:\n%s", code, out.String())
	}

	stable, err := readAllow(strings.NewReader(list), "linux", "go1.27.4")
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run(strings.NewReader(events()), &out, stable, mod); code != 1 || !strings.Contains(out.String(), "NOT ALLOWED a TestNeedsRoot") {
		t.Errorf("on go1.27 the same skip must be NOT ALLOWED, got exit %d:\n%s", code, out.String())
	}

	// Three fields still mean any Go version (the original format).
	if !stable["b TestOptIn"] {
		t.Error("an unqualified line must allow its skip on every Go version")
	}
	if _, err := readAllow(strings.NewReader("linux go1.16 a\n"), "linux", "go1.16.1"); err == nil {
		t.Error("a qualified line without a test must be refused")
	}
}
