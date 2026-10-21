package main

import (
	"bytes"
	"os"

	"github.com/Avalanche-io/c4"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A stand-in tool: prints its arguments and stdin on stdout, a line on
// stderr, and exits with TOOL_EXIT. Compiled, so the same test runs on
// Windows, where a shell script on PATH would not.
const standInTool = `package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

func main() {
	in, _ := io.ReadAll(os.Stdin)
	fmt.Printf("args=%q stdin=%q\n", os.Args[1:], string(in))
	fmt.Fprintln(os.Stderr, "stderr from tool")
	code := 0
	if v := os.Getenv("TOOL_EXIT"); v != "" {
		code, _ = strconv.Atoi(v)
	}
	os.Exit(code)
}
`

// buildTools compiles the stand-in as c4-<name> for each name into one
// directory and returns it.
func buildTools(t *testing.T, names ...string) string {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(standInTool), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module standin\n\ngo 1.16\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	for _, name := range names {
		out := filepath.Join(dir, "c4-"+name+ext)
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = src
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build stand-in %s: %v\n%s", name, err, b)
		}
	}
	return dir
}

type toolRun struct {
	stdout, stderr string
	code           int
}

// runWith runs the c4 binary with PATH set to exactly the given dirs, in
// workdir, with stdin, and a private store (the bare shortcut stores).
func runWith(t *testing.T, bin string, pathDirs []string, workdir, stdin string, extraEnv []string, args ...string) toolRun {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = workdir
	cmd.Stdin = strings.NewReader(stdin)
	env := []string{
		"PATH=" + strings.Join(pathDirs, string(os.PathListSeparator)),
		"C4_STORE=" + t.TempDir(),
	}
	for _, kv := range os.Environ() {
		key := strings.SplitN(kv, "=", 2)[0]
		if strings.EqualFold(key, "PATH") || key == "C4_STORE" {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env, extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("exec: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return toolRun{stdout.String(), stderr.String(), code}
}

// goBinDir is where the go tool lives, kept on PATH so the stand-in
// builds and the scan verbs run unchanged.
func goBinDir(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(goBin)
}

func TestExternalToolPassThrough(t *testing.T) {
	c4 := buildC4(t)
	tools := buildTools(t, "farm")
	path := []string{tools, goBinDir(t)}
	work := t.TempDir()

	r := runWith(t, c4, path, work, "piped input", nil, "farm", "submit", "--farm", "studio", "shot.c4m")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	want := `args=["submit" "--farm" "studio" "shot.c4m"] stdin="piped input"` + "\n"
	if r.stdout != want {
		t.Errorf("stdout %q, want %q", r.stdout, want)
	}
	if r.stderr != "stderr from tool\n" {
		t.Errorf("stderr %q", r.stderr)
	}

	r = runWith(t, c4, path, work, "", []string{"TOOL_EXIT=7"}, "farm", "ls")
	if r.code != 7 {
		t.Errorf("exit %d, want the tool's 7", r.code)
	}
}

// In 1.0.x nothing is reserved: an existing path keeps the bare shortcut,
// exactly as before the dispatch existed.
func TestExternalToolYieldsToExistingPath(t *testing.T) {
	c4 := buildC4(t)
	tools := buildTools(t, "farm", "zap")
	path := []string{tools, goBinDir(t)}
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "farm"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "farm", "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "zap"), []byte("zap\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"farm", "-x"}, {"zap", "-x"}, {"./farm", "-x"}, {"id", "farm"}} {
		r := runWith(t, c4, path, work, "", nil, args...)
		if r.code != 0 {
			t.Errorf("%v: exit %d, stderr %q", args, r.code, r.stderr)
		}
		if strings.Contains(r.stdout, "args=") {
			t.Errorf("%v ran the tool instead of identifying the path: %q", args, r.stdout)
		}
	}

	// No such path: the tool runs.
	r := runWith(t, c4, path, work, "", nil, "zap2")
	if r.code == 0 && strings.Contains(r.stdout, "args=") {
		t.Errorf("zap2 has no tool and should not have run one: %q", r.stdout)
	}
	if err := os.Remove(filepath.Join(work, "zap")); err != nil {
		t.Fatal(err)
	}
	r = runWith(t, c4, path, work, "", nil, "zap", "x")
	if r.code != 0 || !strings.HasPrefix(r.stdout, `args=["x"]`) {
		t.Errorf("c4 zap without a zap path: exit %d, stdout %q", r.code, r.stdout)
	}

	// A name with a path separator is never a tool.
	r = runWith(t, c4, path, work, "", nil, "zap/missing")
	if strings.Contains(r.stdout, "args=") {
		t.Errorf("zap/missing ran a tool: %q", r.stdout)
	}
}

// With no tool and no path, 1.0.x behaves exactly as 1.0.19 did: a piped
// stdin is identified (and stored), and the arguments are ignored. The
// dispatch adds nothing here.
func TestExternalToolMissingKeepsOldBehaviour(t *testing.T) {
	c4bin := buildC4(t)
	path := []string{t.TempDir(), goBinDir(t)}
	work := t.TempDir()

	r := runWith(t, c4bin, path, work, "", nil, "farm", "up")
	want := c4.Identify(strings.NewReader("")).String() + "\n"
	if r.code != 0 || r.stdout != want {
		t.Errorf("exit %d, stdout %q, want the ID of the empty stdin %q", r.code, r.stdout, want)
	}
}

func TestHelpListsInstalledTools(t *testing.T) {
	c4 := buildC4(t)
	tools := buildTools(t, "farm", "zap")
	path := []string{tools, goBinDir(t)}
	work := t.TempDir()

	for _, arg := range []string{"help", "--help", "-h"} {
		r := runWith(t, c4, path, work, "", nil, arg)
		if r.code != 0 {
			t.Fatalf("%s: exit %d, stderr %q", arg, r.code, r.stderr)
		}
		for _, want := range []string{"Usage:", "c4 farm", "c4 zap"} {
			if !strings.Contains(r.stdout, want) {
				t.Errorf("%s lacks %q:\n%s", arg, want, r.stdout)
			}
		}
	}
	r := runWith(t, c4, []string{t.TempDir(), goBinDir(t)}, work, "", nil, "help")
	if !strings.Contains(r.stdout, "not installed") {
		t.Errorf("help without c4-farm should hint at installing it:\n%s", r.stdout)
	}
}

// c4 version and c4 help narrate, on stderr, any other c4 on PATH: the
// ones that come first (and so run when the user types c4) and the ones
// after. With one c4 on PATH they say nothing.
func TestVersionWarnsAboutOtherC4OnPath(t *testing.T) {
	c4bin := buildC4(t)
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	copyTo := func(dir string) string {
		dst := filepath.Join(dir, "c4"+ext)
		data, err := os.ReadFile(c4bin)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0755); err != nil {
			t.Fatal(err)
		}
		return dst
	}
	first := t.TempDir()
	mine := t.TempDir()
	later := t.TempDir()
	shadow := copyTo(first)
	me := copyTo(mine)
	tail := copyTo(later)
	work := t.TempDir()

	// Only this c4 on PATH: no narration.
	r := runWith(t, me, []string{mine, goBinDir(t)}, work, "", nil, "version")
	if r.code != 0 || !strings.HasPrefix(r.stdout, "c4 ") || r.stderr != "" {
		t.Errorf("alone: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}

	// One before, one after.
	for _, verb := range []string{"version", "help"} {
		r = runWith(t, me, []string{first, mine, later, goBinDir(t)}, work, "", nil, verb)
		if r.code != 0 {
			t.Fatalf("%s: exit %d", verb, r.code)
		}
		if !strings.Contains(r.stderr, "comes first on PATH") || !strings.Contains(r.stderr, shadow) {
			t.Errorf("%s: stderr should name the shadowing c4 %s: %q", verb, shadow, r.stderr)
		}
		if !strings.Contains(r.stderr, "later on PATH") || !strings.Contains(r.stderr, tail) {
			t.Errorf("%s: stderr should name the later c4 %s: %q", verb, tail, r.stderr)
		}
		if strings.Contains(r.stdout, "PATH and runs") {
			t.Errorf("%s: the narration leaked into stdout: %q", verb, r.stdout)
		}
	}
}
