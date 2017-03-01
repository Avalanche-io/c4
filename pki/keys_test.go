package pki_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/cheekybits/is"

	c4id "github.com/Avalanche-io/c4/id"
	c4pki "github.com/Avalanche-io/c4/pki"
)

func TestKeys(t *testing.T) {
	is := is.New(t)
	// ent, err := c4pki.NewEntity("john.doe@example.com", c4pki.EMail)
	ent, err := c4pki.NewUser("john.doe@example.com", c4pki.EMail)
	is.NoErr(err)
	is.NotNil(ent)
	err = ent.GenerateKeys()
	is.NoErr(err)

	puk := ent.Public()
	is.NotNil(puk)

	prk := ent.Private()
	is.NotNil(prk)

	fmt.Printf("private key pem: \n%s\n", prk.PEM())

	doc := []byte("foo")
	id, err := c4id.Identify(bytes.NewReader(doc))
	is.NoErr(err)
	is.NotNil(id)

	sig, err := ent.Sign(id)
	is.NoErr(err)
	is.NotNil(sig)

	ent2, err := c4pki.NewUser("john.doe@example.com", c4pki.EMail)
	pubKey := ent.Public()
	ent2.SetPublic(pubKey)
	fmt.Printf("public key pem: \n%s\n", pubKey.PEM())

	is.True(ent2.Verify(sig))

	ent3, err := c4pki.NewUser("jane.doe@example.com", c4pki.EMail)
	is.NoErr(err)
	is.NotNil(ent)
	err = ent3.GenerateKeys()
	is.NoErr(err)

	is.False(ent3.Verify(sig))
}
