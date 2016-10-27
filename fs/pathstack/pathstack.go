package pathstack

import (
	"os"
	"strings"
)

// The PathStack type.
type PathStack []string

// New creates a new PathStack
func New() *PathStack {
	return &PathStack{}
}

// Len returns the number of paths in the stack.
func (s *PathStack) Len() int {
	return len(*s)
}

// Push takes a path string and pushes onto the stack.
func (s *PathStack) Push(path string) {
	*s = append(PathStack{path}, *s...)
}

// Pop removes the top path on the stack and returns it as a string.
func (s *PathStack) Pop() (last string) {
	last = (*s)[0]
	*s = (*s)[1:]
	return
}

// Peek returns the top path on the stack without removing it from the stack.
func (s *PathStack) Peek() (last string) {
	last = (*s)[0]
	return
}

// IsChild returns true if the top path on the stack is a child
// of the path argument.
func (s *PathStack) IsChild(path string) bool {
	last := s.Peek()
	if l := len(last); l <= len(path) || l == 0 {
		return false
	}
	if strings.HasPrefix(last, path) {
		return true
	}
	return false
}

// IsParent returns true if the top path on the stack is any ancestor
// of the path argument.
func (s *PathStack) IsParent(path string) bool {
	last := s.Peek()
	if l := len(path); l <= len(last) || l == 0 {
		return false
	}
	if strings.HasPrefix(path, last) {
		return true
	}
	return false
}

// CommonRoot returns the path common to the top path on the stack
// and the path argument, or an empty string if they have no
// common root.
func (s *PathStack) CommonRoot(path string) string {
	if s.IsParent(path) {
		return s.Peek()
	}
	last := s.Peek()
	lastDirs := strings.Split(last, string(os.PathSeparator))
	pathDirs := strings.Split(path, string(os.PathSeparator))
	commonDirs := []string{}
	for i, d := range lastDirs {
		if len(pathDirs) <= i || pathDirs[i] != d {
			break
		}
		commonDirs = append(commonDirs, d)
	}
	return strings.Join(commonDirs, string(os.PathSeparator))
}

// PopDiff computes a depth ordered difference of the top
// path on the stack and the path argument and returns
// it as a PathStack.
//
// This is useful when walking a directory structure  to do
// computation on each directory as its finish.
func (s *PathStack) PopDiff(path string) *PathStack {
	root := s.CommonRoot(path)
	last := s.Peek()
	lastDirs := strings.Split(last, string(os.PathSeparator))
	ps := New()
	for i, _ := range lastDirs {
		path := strings.Join(lastDirs[:i+1], string(os.PathSeparator))
		if strings.HasPrefix(root, path) {
			continue
		}
		ps.Push(path)
	}
	return ps
}
