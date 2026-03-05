package gitignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		line     string
		wantOK   bool
		negate   bool
		dirOnly  bool
		anchored bool
		segments []string
	}{
		{"", false, false, false, false, nil},
		{"# comment", false, false, false, false, nil},
		{"*.log", true, false, false, false, []string{"*.log"}},
		{"!important.log", true, true, false, false, []string{"important.log"}},
		{"build/", true, false, true, false, []string{"build"}},
		{"/root-only", true, false, false, true, []string{"root-only"}},
		{"src/generated/", true, false, true, true, []string{"src", "generated"}},
		{"**/*.o", true, false, false, true, []string{"**", "*.o"}},
		{"trailing   ", true, false, false, false, []string{"trailing"}},
	}
	for _, tt := range tests {
		p, ok := parseLine(tt.line)
		if ok != tt.wantOK {
			t.Errorf("parseLine(%q): got ok=%v, want %v", tt.line, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if p.negate != tt.negate {
			t.Errorf("parseLine(%q): negate=%v, want %v", tt.line, p.negate, tt.negate)
		}
		if p.dirOnly != tt.dirOnly {
			t.Errorf("parseLine(%q): dirOnly=%v, want %v", tt.line, p.dirOnly, tt.dirOnly)
		}
		if p.anchored != tt.anchored {
			t.Errorf("parseLine(%q): anchored=%v, want %v", tt.line, p.anchored, tt.anchored)
		}
		if strings.Join(p.segments, "/") != strings.Join(tt.segments, "/") {
			t.Errorf("parseLine(%q): segments=%v, want %v", tt.line, p.segments, tt.segments)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"*.log", "error.log", true},
		{"*.log", "error.txt", false},
		{"test?", "test1", true},
		{"test?", "test", false},
		{"test?", "test12", false},
		{"[abc]", "a", true},
		{"[abc]", "d", false},
		{"[a-z]", "m", true},
		{"[a-z]", "M", false},
		{"[!a-z]", "M", true},
		{"[!a-z]", "m", false},
		{"foo", "foo", true},
		{"foo", "bar", false},
		{"\\*", "*", true},
		{"\\*", "a", false},
	}
	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestMatcherBasic(t *testing.T) {
	m := New()
	m.AddFromReader(strings.NewReader(`
# Build artifacts
*.o
*.log
build/
!important.log
`), 0)

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"main.o", false, true},
		{"src/main.o", false, true},
		{"error.log", false, true},        // matches *.log
		{"important.log", false, false},  // negated
		{"build", true, true},            // dir-only pattern
		{"build", false, false},          // not a dir — dir-only pattern doesn't match
		{"src/build", true, true},        // unanchored — matches anywhere
		{"readme.md", false, false},
	}
	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestMatcherAnchored(t *testing.T) {
	m := New()
	m.AddFromReader(strings.NewReader(`
/root-only.txt
src/generated/
`), 0)

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"root-only.txt", false, true},
		{"sub/root-only.txt", false, false},  // anchored to root
		{"src/generated", true, true},
		{"other/src/generated", true, false}, // anchored
	}
	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestMatcherDoublestar(t *testing.T) {
	m := New()
	m.AddFromReader(strings.NewReader(`
**/logs
logs/**
src/**/test
`), 0)

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"logs", false, true},           // **/logs
		{"a/logs", false, true},         // **/logs
		{"a/b/logs", false, true},       // **/logs
		{"logs/debug.log", false, true}, // logs/**
		{"logs/a/b.log", false, true},   // logs/**
		{"src/test", false, true},       // src/**/test (** matches zero)
		{"src/a/test", false, true},     // src/**/test
		{"src/a/b/test", false, true},   // src/**/test
	}
	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestMatcherNested(t *testing.T) {
	// Simulate root .gitignore + subdirectory .gitignore
	m := New()
	m.AddFromReader(strings.NewReader(`
*.log
`), 0) // root

	m.AddFromReader(strings.NewReader(`
!debug.log
`), 1) // src/

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"error.log", false, true},      // matches root *.log
		{"src/error.log", false, true},  // matches root *.log
		{"src/debug.log", false, false}, // negated by src/.gitignore
	}
	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestLoadForPath(t *testing.T) {
	// Create temp directory structure with .gitignore files
	tmp := t.TempDir()

	// Root .gitignore
	os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("*.log\n"), 0644)

	// Sub .gitignore
	sub := filepath.Join(tmp, "src")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("!debug.log\n"), 0644)

	m := LoadForPath(tmp, sub)

	if !m.Match("error.log", false) {
		t.Error("expected error.log to be ignored")
	}
	if m.Match("src/debug.log", false) {
		t.Error("expected src/debug.log to NOT be ignored (negated)")
	}
}

func TestMatcherEmpty(t *testing.T) {
	m := New()
	if m.Match("anything", false) {
		t.Error("empty matcher should match nothing")
	}
}

func TestCommonPatterns(t *testing.T) {
	// Test patterns commonly found in real .gitignore files
	m := New()
	m.AddFromReader(strings.NewReader(`
node_modules/
.env
*.pyc
__pycache__/
.DS_Store
*.swp
dist/
coverage/
.idea/
*.test
`), 0)

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"node_modules", true, true},
		{"src/node_modules", true, true},
		{".env", false, true},
		{"src/.env", false, true},
		{"main.pyc", false, true},
		{"pkg/main.pyc", false, true},
		{"__pycache__", true, true},
		{".DS_Store", false, true},
		{"sub/.DS_Store", false, true},
		{"file.swp", false, true},
		{"dist", true, true},
		{"coverage", true, true},
		{".idea", true, true},
		{"main.test", false, true},
		{"main.go", false, false},
		{"README.md", false, false},
	}
	for _, tt := range tests {
		got := m.Match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}
