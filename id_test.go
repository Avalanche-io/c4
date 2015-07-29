package c4_test

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4go"
)

var _ io.Writer = (*c4.IDEncoder)(nil)
var _ fmt.Stringer = (*c4.ID)(nil)

func TestIDEncoder(t *testing.T) {
	is := is.New(t)
	e := c4.NewIDEncoder()
	is.OK(e)
	_, err := io.Copy(e, strings.NewReader(`This is a pretend asset file, for testing asset id generation.
`))
	is.NoErr(err)

	id := e.ID()
	is.OK(id)
	is.Equal(id.String(), `c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`)

}

func TestParseBytesID(t *testing.T) {
	is := is.New(t)
	e := c4.NewIDEncoder()
	is.OK(e)
	_, err := io.Copy(e, strings.NewReader(`This is a pretend asset file, for testing asset id generation.
`))
	is.NoErr(err)

	id, err := c4.ParseBytesID([]byte(`c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`))
	is.NoErr(err)
	is.Equal(id, e.ID())

	id2, err := c4.ParseID(`c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`)
	is.NoErr(err)
	is.Equal(id2, e.ID())
}
