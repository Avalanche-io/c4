package lang_test

import (
	"strings"
	"testing"

	"github.com/cheekybits/is"
	// c4 "github.com/avalanche-io/c4/id"
	lang "github.com/avalanche-io/c4/lang"
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
	graph, err := lang.YamlLoad(strings.NewReader(yaml1))
	is.NoErr(err)
	is.NotNil(graph)
	// root := graph.Root()
	// is.Equal(root, "yaml test")

}
