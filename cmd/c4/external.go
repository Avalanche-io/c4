package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// External tools: `c4 <name>` runs `c4-<name>` from PATH, the way git
// and kubectl find plugins (design/external-tools.md). The built-in
// verbs win; then a reserved name always dispatches, path or no path;
// then an existing path keeps the bare shortcut; then any other name
// dispatches when its tool is installed.

// reservedTools always mean the tool, even when a file or directory of
// that name exists: the standard release ships them, and `c4 farm` in a
// script or a guide must not depend on the working directory.
var reservedTools = []string{"farm", "agent"}

const toolInstallHint = "install it with the c4 suite (brew install mrjoshuak/tap/c4) or from https://github.com/Avalanche-io/c4-operator/releases"

func isReservedTool(name string) bool {
	for _, r := range reservedTools {
		if name == r {
			return true
		}
	}
	return false
}

// externalTool decides whether `c4 <name>` runs an external tool. It
// returns the tool's path and whether to run it; reserved reports a
// reserved name, which is an error when its tool is missing rather than
// a fall-through to the path shortcut.
func externalTool(name string) (bin string, reserved bool, run bool) {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsAny(name, `/\`) {
		return "", false, false
	}
	reserved = isReservedTool(name)
	if !reserved {
		if _, err := os.Stat(name); err == nil {
			return "", false, false
		}
	}
	bin, err := exec.LookPath("c4-" + name)
	if err != nil {
		return "", reserved, false
	}
	return bin, reserved, true
}

// runExternal runs the tool with c4's stdin, stdout, stderr and
// environment and returns its exit code. An interrupt from the terminal
// reaches the tool directly; c4 only waits for it, so the exit code is
// the tool's.
func runExternal(bin string, args []string) int {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		return 1
	}
	fmt.Fprintf(os.Stderr, "c4: %s: %v\n", filepath.Base(bin), err)
	return 1
}

// installedTools lists every c4-<name> executable on PATH, each name
// once, in sorted order, with the first match's path.
func installedTools() (names []string, paths map[string]string) {
	paths = map[string]string{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := toolName(e.Name())
			if name == "" {
				continue
			}
			if _, seen := paths[name]; seen {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if !isExecutable(full) {
				continue
			}
			paths[name] = full
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, paths
}

// toolName turns a file name into a tool name, or "" when the file is
// not a c4-<name> tool. On Windows the extension is dropped.
func toolName(file string) string {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(file))
		if ext == ".exe" || ext == ".cmd" || ext == ".bat" {
			file = file[:len(file)-len(ext)]
		}
	}
	if !strings.HasPrefix(file, "c4-") || len(file) == len("c4-") {
		return ""
	}
	return file[len("c4-"):]
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

// printInstalledTools follows the top help page: the installed tools, and
// the reserved ones that are missing.
func printInstalledTools(w io.Writer) {
	names, paths := installedTools()
	fmt.Fprintln(w, "Tools (c4 <name> runs c4-<name> from PATH; c4 farm and c4 agent always do):")
	for _, name := range names {
		fmt.Fprintf(w, "  c4 %-12s %s\n", name, paths[name])
	}
	for _, name := range reservedTools {
		if _, ok := paths[name]; !ok {
			fmt.Fprintf(w, "  c4 %-12s not installed: %s\n", name, toolInstallHint)
		}
	}
}

// otherC4s finds every other c4 executable on PATH: the ones before the
// running binary (they win when the user types c4) and the ones after.
// A dev build in ~/bin ahead of Homebrew's c4 was invisible until c4
// farm did nothing.
func otherC4s() (before, after []string) {
	self, err := os.Executable()
	if err != nil {
		return nil, nil
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	name := "c4"
	if runtime.GOOS == "windows" {
		name = "c4.exe"
	}
	seenSelf := false
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		if !isExecutable(candidate) {
			continue
		}
		resolved := candidate
		if r, err := filepath.EvalSymlinks(candidate); err == nil {
			resolved = r
		}
		if resolved == self {
			seenSelf = true
			continue
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		if seenSelf {
			after = append(after, candidate)
		} else {
			before = append(before, candidate)
		}
	}
	return before, after
}

// warnOtherC4s narrates, on w, any other c4 on PATH. Called by c4 version
// and c4 help; it prints nothing when this binary is the only c4.
func warnOtherC4s(w io.Writer) {
	before, after := otherC4s()
	self, _ := os.Executable()
	for _, p := range before {
		fmt.Fprintf(w, "c4: another c4 comes first on PATH and runs when you type c4: %s (this is %s)\n", p, self)
	}
	for _, p := range after {
		fmt.Fprintf(w, "c4: another c4 is later on PATH: %s\n", p)
	}
}
