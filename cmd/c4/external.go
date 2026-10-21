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
// and kubectl find plugins. In the 1.0.x line this is purely additive:
// the built-in verbs win, then an existing file or directory keeps the
// bare shortcut, and only a name that is neither runs its tool. (The
// v1.1 rule reserves `farm` and `agent` so they win over a same-named
// directory; a patch release may not change what an existing invocation
// does, so 1.0.x does not reserve anything.)

const toolInstallHint = "install it with the c4 suite (brew install mrjoshuak/tap/c4) or from https://github.com/Avalanche-io/c4-operator/releases"

// externalTool decides whether `c4 <name>` runs an external tool: the
// name is not a flag, has no path separator, names no existing file or
// directory, and c4-<name> is on PATH.
func externalTool(name string) (bin string, run bool) {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsAny(name, `/\`) {
		return "", false
	}
	if _, err := os.Stat(name); err == nil {
		return "", false
	}
	bin, err := exec.LookPath("c4-" + name)
	if err != nil {
		return "", false
	}
	return bin, true
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

// printInstalledTools follows the help page: the installed tools, and a
// hint when c4-farm is not among them.
func printInstalledTools(w io.Writer) {
	names, paths := installedTools()
	fmt.Fprintln(w, "Tools (c4 <name> runs c4-<name> from PATH when no file or directory <name> exists):")
	for _, name := range names {
		fmt.Fprintf(w, "  c4 %-12s %s\n", name, paths[name])
	}
	if _, ok := paths["farm"]; !ok {
		fmt.Fprintf(w, "  c4 %-12s not installed: %s\n", "farm", toolInstallHint)
	}
}
