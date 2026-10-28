package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
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

// runWith runs the c4 binary with PATH set to exactly the given dirs,
// in workdir, with stdin.
func runWith(t *testing.T, bin string, pathDirs []string, workdir, stdin string, extraEnv []string, args ...string) toolRun {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = workdir
	cmd.Stdin = strings.NewReader(stdin)
	env := []string{"PATH=" + strings.Join(pathDirs, string(os.PathListSeparator))}
	for _, kv := range os.Environ() {
		key := strings.SplitN(kv, "=", 2)[0]
		if strings.EqualFold(key, "PATH") {
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

func TestExternalToolReservedNameWinsOverDirectory(t *testing.T) {
	c4 := buildC4(t)
	tools := buildTools(t, "farm")
	path := []string{tools, goBinDir(t)}
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "farm"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "farm", "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// c4 farm runs the tool, even though ./farm exists.
	r := runWith(t, c4, path, work, "", nil, "farm", "status")
	if r.code != 0 || !strings.HasPrefix(r.stdout, `args=["status"]`) {
		t.Errorf("c4 farm with a farm directory: exit %d, stdout %q", r.code, r.stdout)
	}

	// The directory is still reachable as a path, both ways.
	for _, args := range [][]string{{"./farm"}, {"id", "farm"}} {
		r = runWith(t, c4, path, work, "", nil, args...)
		if r.code != 0 {
			t.Errorf("%v: exit %d, stderr %q", args, r.code, r.stderr)
		}
		if strings.Contains(r.stdout, "args=") || !strings.Contains(r.stdout, "a.txt") {
			t.Errorf("%v did not scan the directory: %q", args, r.stdout)
		}
	}
}

func TestExternalToolUnreservedNameYieldsToPaths(t *testing.T) {
	c4 := buildC4(t)
	tools := buildTools(t, "zap")
	path := []string{tools, goBinDir(t)}
	work := t.TempDir()

	// No path named zap: c4 zap runs c4-zap.
	r := runWith(t, c4, path, work, "", nil, "zap", "x")
	if r.code != 0 || !strings.HasPrefix(r.stdout, `args=["x"]`) {
		t.Errorf("c4 zap: exit %d, stdout %q", r.code, r.stdout)
	}

	// A file named zap: today's shortcut, the file is identified.
	if err := os.WriteFile(filepath.Join(work, "zap"), []byte("zap\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r = runWith(t, c4, path, work, "", nil, "zap")
	if r.code != 0 {
		t.Errorf("c4 zap with a zap file: exit %d, stderr %q", r.code, r.stderr)
	}
	if strings.Contains(r.stdout, "args=") || !strings.Contains(r.stdout, " zap ") {
		t.Errorf("c4 zap with a zap file did not identify the file: %q", r.stdout)
	}

	// A name with a path separator is never a tool.
	r = runWith(t, c4, path, work, "", nil, "zap/missing")
	if strings.Contains(r.stdout, "args=") {
		t.Errorf("zap/missing ran a tool: %q", r.stdout)
	}
}

func TestExternalToolMissingReservedTool(t *testing.T) {
	c4 := buildC4(t)
	path := []string{t.TempDir(), goBinDir(t)}
	work := t.TempDir()

	r := runWith(t, c4, path, work, "", nil, "farm", "up")
	if r.code != 1 {
		t.Errorf("exit %d, want 1", r.code)
	}
	if r.stdout != "" {
		t.Errorf("stdout should be empty, got %q", r.stdout)
	}
	if !strings.Contains(r.stderr, "c4-farm") || !strings.Contains(r.stderr, "PATH") {
		t.Errorf("stderr should name c4-farm and PATH: %q", r.stderr)
	}
}

func TestHelpListsInstalledTools(t *testing.T) {
	c4 := buildC4(t)
	tools := buildTools(t, "farm", "zap")
	path := []string{tools, goBinDir(t)}
	work := t.TempDir()

	r := runWith(t, c4, path, work, "", nil, "help")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	for _, want := range []string{"c4 farm", "c4 zap", "c4 agent", "not installed"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help lacks %q:\n%s", want, r.stdout)
		}
	}
	if strings.Index(r.stdout, "c4 farm         "+tools) > strings.Index(r.stdout, "c4 zap          "+tools) {
		t.Errorf("tools are not sorted:\n%s", r.stdout)
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

// c4 -s -S on a tree whose sequence sits in a subdirectory must store every
// frame. Through 1.0.19 it warned "cannot read sequence member" for each
// frame and stored none, because the folded manifest listed the range
// before its parent directory.
func TestSnapshotStoresNestedSequenceFrames(t *testing.T) {
	c4bin := buildC4(t)
	work := t.TempDir()
	plates := filepath.Join(work, "comp", "plates")
	if err := os.MkdirAll(plates, 0755); err != nil {
		t.Fatal(err)
	}
	var frames []string
	for i := 1001; i <= 1004; i++ {
		content := fmt.Sprintf("plate %d", i)
		frames = append(frames, content)
		if err := os.WriteFile(filepath.Join(plates, fmt.Sprintf("plate.%d.exr", i)), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	store := t.TempDir()
	r := runWith(t, c4bin, []string{goBinDir(t)}, work, "", []string{"C4_STORE=" + store}, "-s", "-S", "comp/")
	if r.code != 0 {
		t.Fatalf("c4 -S: exit %d, stderr %q", r.code, r.stderr)
	}
	if strings.Contains(r.stderr, "cannot read sequence member") || strings.Contains(r.stderr, "was not stored") {
		t.Fatalf("frames were not stored:\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "plate.[1001-1004].exr") {
		t.Fatalf("sequence was not folded:\n%s", r.stdout)
	}
	for _, content := range frames {
		id := c4.Identify(strings.NewReader(content)).String()
		got := runWith(t, c4bin, []string{goBinDir(t)}, work, "", []string{"C4_STORE=" + store}, "cat", id)
		if got.code != 0 || got.stdout != content {
			t.Errorf("frame %q not in the store: exit %d, stdout %q, stderr %q", content, got.code, got.stdout, got.stderr)
		}
	}
}
