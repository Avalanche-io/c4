// Command skips reads `go test -json` on stdin and fails when a test
// skipped that the allow-list does not name, so a test that runs nowhere
// cannot go unnoticed. It prints what `go test` prints for each package
// (ok, FAIL, no test files), the output of every test that failed, and the
// skips it found, each with the reason the test gave.
//
//	go test -json ./... | go run ./hack/skips -allow hack/skips/allowed.txt
//
// The allow-list holds one skip per line: the GOOS it is allowed on (or
// "*"), the package relative to the module, and the test, with a reason
// after "#". It exits 1 when a test or a build failed, or a skip is not
// listed; a listed skip that did not happen is reported, not failed on,
// since a skip depends on what the machine has.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
)

// event is one line of `go test -json` (cmd/test2json).
type event struct {
	Action  string
	Package string
	Test    string
	Output  string
	Elapsed float64
}

// skip is a test that skipped, and the reason it gave.
type skip struct{ pkg, test, reason string }

func main() {
	allow := flag.String("allow", "", "the allow-list")
	module := flag.String("module", "github.com/Avalanche-io/c4", "the module, stripped from package paths")
	goos := flag.String("goos", runtime.GOOS, "the GOOS the tests ran on")
	report := flag.Bool("report", false, "report unlisted skips without failing on them (to measure a new leg)")
	flag.Parse()
	allowed := map[string]bool{}
	if *allow != "" {
		f, err := os.Open(*allow)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		allowed, err = readAllow(f, *goos)
		f.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	reportOnly = *report
	os.Exit(run(os.Stdin, os.Stdout, allowed, *module))
}

// readAllow returns the skips the list allows on goos, as "pkg test".
func readAllow(r io.Reader, goos string) (map[string]bool, error) {
	out := map[string]bool{}
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line, _, _ := cutString(sc.Text(), "#")
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) != 3 {
			return nil, fmt.Errorf("allow-list line %d: want GOOS, package and test, have %q", n, sc.Text())
		}
		if f[0] == "*" || f[0] == goos {
			out[f[1]+" "+f[2]] = true
		}
	}
	return out, sc.Err()
}

// run reads the events, prints the summary to w, and returns the exit code.
func run(r io.Reader, w io.Writer, allowed map[string]bool, module string) int {
	short := func(pkg string) string {
		if pkg == module {
			return "."
		}
		return strings.TrimPrefix(pkg, module+"/")
	}
	output := map[string][]string{} // "pkg test" to its output lines
	ran := map[string]bool{}        // packages in the stream
	open := map[string]bool{}       // packages started and not yet ended
	var skips []skip
	failed := false
	events := 0
	dec := json.NewDecoder(r)
	for {
		var e event
		if err := dec.Decode(&e); err == io.EOF {
			break
		} else if err != nil {
			fmt.Fprintf(w, "skips: reading go test -json: %v\n", err)
			return 1
		}
		events++
		key := e.Package + " " + e.Test
		if e.Package != "" {
			ran[short(e.Package)] = true
		}
		if e.Test == "" && e.Package != "" {
			switch e.Action {
			case "start":
				open[e.Package] = true
			case "pass", "fail", "skip":
				delete(open, e.Package)
			}
		}
		switch e.Action {
		case "output", "build-output":
			if e.Test == "" {
				// A package's own lines: its result, and what a build or
				// a TestMain printed.
				if s := strings.TrimRight(e.Output, "\n"); s != "" && !strings.HasPrefix(s, "=== ") && s != "PASS" && s != "FAIL" {
					fmt.Fprintln(w, s)
				}
				continue
			}
			output[key] = append(output[key], e.Output)
		case "fail", "build-fail":
			failed = true
			if e.Test != "" {
				fmt.Fprint(w, strings.Join(output[key], ""))
			}
		case "skip":
			if e.Test != "" {
				skips = append(skips, skip{short(e.Package), e.Test, reason(output[key])})
			}
		}
		if e.Action != "output" && e.Action != "build-output" && e.Test != "" {
			if e.Action == "pass" || e.Action == "skip" {
				delete(output, key)
			}
		}
	}
	sort.Slice(skips, func(i, j int) bool {
		if skips[i].pkg != skips[j].pkg {
			return skips[i].pkg < skips[j].pkg
		}
		return skips[i].test < skips[j].test
	})
	var unlisted []skip
	seen := map[string]bool{}
	for _, s := range skips {
		seen[s.pkg+" "+s.test] = true
		if !allowed[s.pkg+" "+s.test] {
			unlisted = append(unlisted, s)
		}
	}
	var cut []string
	for p := range open {
		cut = append(cut, short(p))
	}
	sort.Strings(cut)
	for _, p := range cut {
		// A test binary killed or cut off mid-run ends no package line.
		fmt.Fprintf(w, "skips: %s started and never ended\n", p)
		failed = true
	}
	if events == 0 {
		// go test that crashed, or never ran, writes nothing: no tests is not
		// a pass.
		fmt.Fprintln(w, "skips: go test -json wrote no events")
		return 1
	}
	fmt.Fprintf(w, "skips: %d tests skipped, %d of them allowed\n", len(skips), len(skips)-len(unlisted))
	for _, s := range unlisted {
		fmt.Fprintf(w, "skips: NOT ALLOWED %s %s: %s\n", s.pkg, s.test, s.reason)
	}
	var unused []string
	for k := range allowed {
		pkg, _, _ := cutString(k, " ")
		if !seen[k] && ran[pkg] {
			unused = append(unused, k)
		}
	}
	sort.Strings(unused)
	for _, k := range unused {
		fmt.Fprintf(w, "skips: allowed, and it did not skip: %s\n", k)
	}
	if failed || (len(unlisted) > 0 && !reportOnly) {
		return 1
	}
	return 0
}

// reportOnly, set by -report, lists unlisted skips without failing on them,
// to measure what a new CI leg skips before its allow-list is written.
var reportOnly bool

// cutString is strings.Cut, which Go 1.16 (the oldest Go this module builds
// with) does not have.
func cutString(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// reason is what a skipping test gave as its reason: the message a t.Skip
// wrote (file:line: reason), with any lines it ran on to, else the last
// line it printed.
func reason(lines []string) string {
	var msg []string
	for i := len(lines) - 1; i >= 0; i-- {
		raw := lines[i]
		s := strings.TrimSpace(raw)
		if strings.HasPrefix(s, "--- SKIP") || strings.HasPrefix(s, "=== ") || s == "" {
			continue
		}
		if _, after, ok := cutString(s, ".go:"); ok {
			if _, m, ok := cutString(after, ": "); ok {
				return strings.Join(append([]string{m}, msg...), " ")
			}
		}
		// A line a multi-line reason ran on to (testing indents them).
		msg = append([]string{s}, msg...)
	}
	return strings.Join(msg, " ")
}
