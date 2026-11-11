package reconcile

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// A c4m name is any bytes but "/" and NUL (Joshua, 2026-11-10: "C4m files
// only considers forward slash a path seperator. Everything else is a
// literal or encodable character"). Where a platform cannot hold a name,
// a writer refuses that entry, says which and why, and writes the rest: it
// never renames it, maps it or joins it into a path, as git does. Git's
// set is the most a writer may refuse; anything measured to round-trip is
// not refused. The rule and its classes match c4-operator's fsname
// package, and both read c4m/testdata/name-refusal-vector so they cannot
// drift.

// Refusal classes, in the order a name is checked.
const (
	refuseBackslash = "backslash" // windows: git v2.47.0 read-cache.c verify_path_internal
	refuseChar      = "char"      // windows: one of : < > " | ? *, compat/mingw.c is_valid_win32_path
	refuseControl   = "control"   // windows: a byte 0x01-0x1F, is_valid_win32_path
	refuseTrailing  = "trailing"  // windows: a trailing space or dot, is_valid_win32_path (not measured)
	refuseDevice    = "device"    // windows: a reserved device name, is_valid_win32_path (not measured)
	refuseUTF8      = "utf8"      // darwin: not valid UTF-8 (measured on APFS: "Illegal byte sequence")
)

// nameRefusal says why a filesystem on goos cannot hold name (one path
// segment, without a trailing "/") as it is: its class and a reason, or
// "" when it can. Linux holds every byte but "/" and NUL.
func nameRefusal(goos, name string) (class, reason string) {
	switch goos {
	case "windows":
		return windowsRefusal(name)
	case "darwin", "ios":
		if !utf8.ValidString(name) {
			return refuseUTF8, "APFS holds no name that is not UTF-8 (measured)"
		}
	}
	return "", ""
}

// pathRefusal applies nameRefusal to each segment of a slash-separated
// path relative to the destination, and returns the first refused.
func pathRefusal(goos, path string) (segment, reason string) {
	for _, seg := range strings.Split(strings.TrimSuffix(path, "/"), "/") {
		if _, r := nameRefusal(goos, seg); r != "" {
			return seg, r
		}
	}
	return "", ""
}

// targetRefusal says why a symbolic link's target cannot be written as it
// is on goos: on Windows any segment nameRefusal refuses; elsewhere never,
// since a target is bytes kept as they are.
func targetRefusal(goos, target string) (segment, reason string) {
	if goos != "windows" {
		return "", ""
	}
	return pathRefusal(goos, target)
}

// windowsRefusal is git's rule for a path segment Git for Windows will not
// write, as of git v2.47.0. The bytes are checked from the first, each for
// a backslash, then the characters, then a control byte; then a trailing
// space or dot; then the reserved device names. Git's repository guards
// (".git" look-alikes, "GIT~1") are not taken: they protect a repository,
// not a filesystem. The trailing and device rules are git's, not a
// measurement: a writer using \\?\ paths may hold such names.
func windowsRefusal(name string) (class, reason string) {
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '\\':
			return refuseBackslash, "a backslash is a separator on Windows (git v2.47.0, read-cache.c verify_path_internal)"
		case strings.IndexByte(`:<>"|?*`, c) >= 0:
			return refuseChar, fmt.Sprintf("%q is not allowed in a Windows name (git v2.47.0, compat/mingw.c is_valid_win32_path)", c)
		case c > 0 && c < 0x20:
			return refuseControl, fmt.Sprintf("a control character (0x%02x) is not allowed in a Windows name (git v2.47.0, compat/mingw.c is_valid_win32_path)", c)
		}
	}
	if name != "." && name != ".." && (strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".")) {
		return refuseTrailing, "a Windows name cannot end in a space or a dot (git v2.47.0, compat/mingw.c is_valid_win32_path; not measured)"
	}
	if d := windowsDevice(name); d != "" {
		return refuseDevice, fmt.Sprintf("%s is a reserved device name on Windows, with any extension (git v2.47.0, compat/mingw.c is_valid_win32_path; not measured)", d)
	}
	return "", ""
}

// windowsDevice returns the reserved device name name begins with, as git
// matches it: case-insensitively, followed by nothing, spaces, an extension
// or a stream ("." or ":"); COM1 to COM9, LPT0 to LPT9.
func windowsDevice(name string) string {
	stem := strings.ToUpper(name)
	if i := strings.IndexAny(stem, ".:"); i >= 0 {
		stem = stem[:i]
	}
	stem = strings.TrimRight(stem, " ")
	switch stem {
	case "AUX", "CON", "CONIN$", "CONOUT$", "NUL", "PRN":
		return stem
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") && stem[3] >= '1' && stem[3] <= '9' ||
		strings.HasPrefix(stem, "LPT") && stem[3] >= '0' && stem[3] <= '9') {
		return stem
	}
	return ""
}
