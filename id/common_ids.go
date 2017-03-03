package id

import (
	"strings"
)

var (
	// Id of nil (i.e. "")
	NIL_ID *ID
)

func init() {
	NIL_ID = Identify(strings.NewReader(""))
}
