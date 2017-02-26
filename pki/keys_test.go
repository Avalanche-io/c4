package pki_test

import (
	"bytes"
	"testing"

	"github.com/cheekybits/is"

	c4id "github.com/Avalanche-io/c4/id"
	c4pki "github.com/Avalanche-io/c4/pki"
)

func TestKeys(t *testing.T) {
	is := is.New(t)
	ent, err := c4pki.NewEntity("john.doe@example.com", c4pki.EMail)
	is.NoErr(err)
	is.NotNil(ent)

	puk := ent.Public()
	is.NotNil(puk)

	prk := ent.Private()
	is.NotNil(prk)

	doc := []byte("foo")
	id, err := c4id.Identify(bytes.NewReader(doc))
	is.NoErr(err)
	is.NotNil(id)

	sig, err := ent.Sign(id)
	is.NoErr(err)
	is.NotNil(sig)

	ent2 := c4pki.Entity{}
	ent2.SetPublic(ent.Public())

	is.True(ent2.Verify(sig))

	ent3, err := c4pki.NewEntity("jane.doe@example.com", c4pki.EMail)
	is.NoErr(err)
	is.NotNil(ent3)
	is.False(ent3.Verify(sig))
}
