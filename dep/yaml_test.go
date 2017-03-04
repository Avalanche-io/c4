package dep_test

import (
	"testing"

	"github.com/cheekybits/is"
	// c4 "github.com/Avalanche-io/c4/id"
	// dep "github.com/Avalanche-io/c4/dep"
)

var yaml1 string = `
---
yaml test:
	dep1:
		foo:
			bar: 42
`

func TestYamlDep(t *testing.T) {
	is := is.New(t)
	_ = is
	// Load a yaml file
	graph := dep.YamlLoad(strings.NewReader(yaml1))
	root := graph.Root()
	is.Equal(root, "yaml test")

}
