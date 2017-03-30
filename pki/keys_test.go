package pki_test

import (
	"bytes"
	"testing"

	"github.com/cheekybits/is"

	c4id "github.com/avalanche-io/c4/id"
	"github.com/avalanche-io/c4/pki"
)

func TestKeys(t *testing.T) {
	is := is.New(t)
	ent, err := pki.NewUser("john.doe@example.com", pki.EMail)
	is.NoErr(err)
	is.NotNil(ent)
	err = ent.GenerateKeys()
	is.NoErr(err)

	pub := ent.Public()
	is.NotNil(pub)

	pri := ent.Private()
	is.NotNil(pri)

	doc := []byte("foo")
	id := c4id.Identify(bytes.NewReader(doc))
	is.NotNil(id)

	sig, err := ent.Sign(id)
	is.NoErr(err)
	is.NotNil(sig)

	is.True(pub.Varify(sig))

	ent3, err := pki.NewUser("jane.doe@example.com", pki.EMail)
	is.NoErr(err)
	is.NotNil(ent)
	err = ent3.GenerateKeys()
	is.NoErr(err)
	pub = ent3.Public()
	is.False(pub.Varify(sig))
}
