package pathstack_test

import (
	"testing"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/fs/pathstack"
)

func TestPathstackBasic(t *testing.T) {
	is := is.New(t)
	ps := pathstack.New()

	ps.Push("/foo")
	ps.Push("/foo/bar")
	ps.Push("/foo/bar/bat")
	ps.Push("/foo/bar/bat/baz")

	is.Equal(ps.Pop(), "/foo/bar/bat/baz")
	is.Equal(ps.Peek(), "/foo/bar/bat")
	is.Equal(ps.Pop(), "/foo/bar/bat")

	ps.Push("/foo/baz/bat")

	is.Equal(ps.Peek(), "/foo/baz/bat")
	is.Equal(ps.Pop(), "/foo/baz/bat")
	is.Equal(ps.Pop(), "/foo/bar")

}

func TestPathstack(t *testing.T) {
	is := is.New(t)

	tests := []struct {
		Path1      string
		Path2      string
		IsParent   bool
		IsChild    bool
		CommonRoot string
		PopDiff    []string
	}{
		{
			Path1:      "/foo/bar/bat/baz",
			Path2:      "/foo/bar/bat",
			IsParent:   false,
			IsChild:    true,
			CommonRoot: "/foo/bar/bat",
			PopDiff:    []string{"/foo/bar/bat/baz"},
		},
		{
			Path1:      "/foo/bar/bat",
			Path2:      "/foo/bar/bat/baz",
			IsParent:   true,
			IsChild:    false,
			CommonRoot: "/foo/bar/bat",
			PopDiff:    []string{},
		},
		{
			Path1:      "/foo/bar/bat",
			Path2:      "/foo/foo/bat/baz",
			IsParent:   false,
			IsChild:    false,
			CommonRoot: "/foo",
			PopDiff:    []string{"/foo/bar/bat", "/foo/bar"},
		},
	}

	for i, tt := range tests {
		_ = i
		ps := pathstack.New()
		is.NotNil(ps)
		is.NotNil(tt)
		ps.Push(tt.Path1)
		is.Equal(ps.IsParent(tt.Path2), tt.IsParent)
		is.Equal(ps.IsChild(tt.Path2), tt.IsChild)
		is.Equal(ps.CommonRoot(tt.Path2), tt.CommonRoot)

		diff := ps.PopDiff(tt.Path2)
		for _, path := range tt.PopDiff {
			is.Equal(diff.Pop(), path)
		}

	}
}
