package id

import (
	"strings"
)

var (
	// Id of nil (i.e. "")
	NIL_ID *ID

	// Id with all bytes set to 0.
	VOID_ID *ID

	MAX_ID *ID
)

func init() {
	NIL_ID = Identify(strings.NewReader(""))
	var void [64]byte
	VOID_ID = Digest(void[:]).ID()
	var max [64]byte
	for i := range max {
		max[i] = 0xFF
	}
	MAX_ID = Digest(max[:]).ID()
}
