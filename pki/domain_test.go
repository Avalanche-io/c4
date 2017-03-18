package pki_test

import (
	"encoding/json"
	"testing"

	"github.com/cheekybits/is"

	"github.com/Avalanche-io/c4/pki"
)

func TestDomainSaveLoad(t *testing.T) {
	is := is.New(t)

	// Create a Certificate Authority
	ca, err := pki.CreateCA("c4.studio.com")
	is.NoErr(err)
	is.NotNil(ca)

	// u1, err := pki.New("john.doe@example.com", pki.EMail)
	// is.NoErr(err)
	// is.NotNil(u1)
	// is.NoErr(u1.GenerateKeys())

	is.NoErr(ca.Passphrase("some passphrase"))

	// Save
	data, err := json.Marshal(ca)
	is.NoErr(err)

	// Load
	var ca2 pki.Domain
	is.NoErr(json.Unmarshal(data, &ca2))

	// Does not save the PrivateKey, or Passphrase in the clear
	is.NotNil(ca2.EncryptedPrivateKey)
	is.Nil(ca2.ClearPrivateKey)
	is.NotNil(ca2.EncryptedPassphrase)
	is.Nil(ca2.ClearPassphrase)

	is.NoErr(ca2.Passphrase("some passphrase"))
	is.NotNil(ca2.Private())

	var ca3 pki.Domain
	is.NoErr(json.Unmarshal(data, &ca3))

	err = ca3.Passphrase("wrong passphrase")
	is.Err(err)
	is.Equal(err.Error(), "incorrect passphrase")

	is.Nil(ca3.Private())
}
