// Package gitignore implements .gitignore pattern matching for c4 scan.
//
// It supports the full .gitignore specification: glob patterns, directory-only
// patterns, negation, anchored patterns, and ** wildcards. Patterns from
// parent .gitignore files cascade into subdirectories.
package gitignore

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// pattern is a single parsed .gitignore rule.
type pattern struct {
	segments []string
	negate   bool
	dirOnly  bool
	anchored bool
}

// Matcher holds patterns from one or more .gitignore files.
type Matcher struct {
	rules []rule
}

// rule associates a pattern with its source directory depth so that
// anchored patterns resolve correctly relative to their .gitignore.
type rule struct {
	pattern
	depth int // depth of the .gitignore relative to scan root (0 = root)
}

// New creates an empty Matcher.
func New() *Matcher {
	return &Matcher{}
}

// AddFromFile loads patterns from a .gitignore file at the given depth.
// depth is how many directories deep from the scan root (0 = root).
func (m *Matcher) AddFromFile(path string, depth int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	m.AddFromReader(f, depth)
	return nil
}

// AddFromReader loads patterns from a reader at the given depth.
func (m *Matcher) AddFromReader(r io.Reader, depth int) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if p, ok := parseLine(scanner.Text()); ok {
			m.rules = append(m.rules, rule{pattern: p, depth: depth})
		}
	}
}

// Match checks if a path should be ignored. relPath is the slash-separated
// path relative to the scan root (e.g. "src/main.go"). isDir indicates
// whether the entry is a directory.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	if len(m.rules) == 0 {
		return false
	}

	segments := strings.Split(relPath, "/")
	ignored := false

	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}

		// For anchored patterns from a sub-.gitignore, match against the
		// path relative to that .gitignore's directory.
		matchSegs := segments
		if r.depth > 0 && r.depth < len(segments) {
			matchSegs = segments[r.depth:]
		}

		if matchesPattern(r.pattern, matchSegs) {
			ignored = !r.negate
		}
	}
	return ignored
}

func matchesPattern(p pattern, pathSegments []string) bool {
	if p.anchored {
		return matchSegments(p.segments, pathSegments)
	}
	// Unanchored single-segment: match any component
	if len(p.segments) == 1 {
		for _, seg := range pathSegments {
			if globMatch(p.segments[0], seg) {
				return true
			}
		}
		return false
	}
	// Unanchored multi-segment: slide window
	for i := 0; i <= len(pathSegments)-len(p.segments); i++ {
		if matchSegments(p.segments, pathSegments[i:]) {
			return true
		}
	}
	return false
}

// matchSegments matches pattern segments against path segments,
// handling ** (doublestar) for zero or more directories.
func matchSegments(pat, path []string) bool {
	pi, si := 0, 0
	for pi < len(pat) && si < len(path) {
		if pat[pi] == "**" {
			pi++
			if pi == len(pat) {
				return true
			}
			for si <= len(path) {
				if matchSegments(pat[pi:], path[si:]) {
					return true
				}
				si++
			}
			return false
		}
		if !globMatch(pat[pi], path[si]) {
			return false
		}
		pi++
		si++
	}
	for pi < len(pat) && pat[pi] == "**" {
		pi++
	}
	return pi == len(pat) && si == len(path)
}

// globMatch matches a single path segment against a glob pattern.
func globMatch(pattern, name string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			pattern = pattern[1:]
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if globMatch(pattern, name[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(name) == 0 {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		case '[':
			if len(name) == 0 {
				return false
			}
			end := strings.IndexByte(pattern[1:], ']')
			if end < 0 {
				if name[0] != pattern[0] {
					return false
				}
				pattern = pattern[1:]
				name = name[1:]
				continue
			}
			classBody := pattern[1 : end+1]
			pattern = pattern[end+2:]
			negate := false
			if len(classBody) > 0 && (classBody[0] == '!' || classBody[0] == '^') {
				negate = true
				classBody = classBody[1:]
			}
			if matchCharClass(classBody, name[0]) == negate {
				return false
			}
			name = name[1:]
		case '\\':
			pattern = pattern[1:]
			if len(pattern) == 0 {
				return false
			}
			if len(name) == 0 || name[0] != pattern[0] {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		default:
			if len(name) == 0 || name[0] != pattern[0] {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		}
	}
	return len(name) == 0
}

func matchCharClass(class string, ch byte) bool {
	for i := 0; i < len(class); i++ {
		if i+2 < len(class) && class[i+1] == '-' {
			if ch >= class[i] && ch <= class[i+2] {
				return true
			}
			i += 2
			continue
		}
		if class[i] == ch {
			return true
		}
	}
	return false
}

func parseLine(line string) (pattern, bool) {
	line = trimTrailingSpace(line)
	if line == "" || line[0] == '#' {
		return pattern{}, false
	}

	p := pattern{}

	if line[0] == '!' {
		p.negate = true
		line = line[1:]
		if line == "" {
			return pattern{}, false
		}
	}

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimRight(line, "/")
	}

	if strings.HasPrefix(line, "/") {
		p.anchored = true
		line = strings.TrimPrefix(line, "/")
	}

	if !p.anchored && strings.Contains(line, "/") {
		p.anchored = true
	}

	p.segments = strings.Split(line, "/")
	return p, true
}

func trimTrailingSpace(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		if len(s) > 1 && s[len(s)-2] == '\\' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// LoadForPath loads all .gitignore files from scanRoot down to dirPath.
func LoadForPath(scanRoot, dirPath string) *Matcher {
	m := New()
	m.AddFromFile(filepath.Join(scanRoot, ".gitignore"), 0)

	rel, err := filepath.Rel(scanRoot, dirPath)
	if err != nil || rel == "." {
		return m
	}

	parts := strings.Split(rel, string(filepath.Separator))
	current := scanRoot
	for i, part := range parts {
		current = filepath.Join(current, part)
		m.AddFromFile(filepath.Join(current, ".gitignore"), i+1)
	}
	return m
}
