package pki_test

import (
	"crypto/x509/pkix"
	"encoding/json"
	"testing"

	"github.com/cheekybits/is"

	"github.com/Avalanche-io/c4/pki"
)

// updated, delete me

func TestDomainSaveLoad(t *testing.T) {
	is := is.New(t)

	// Create a Certificate Authority
	ca, err := pki.CreateAthorty(pkix.Name{CommonName: "c4.studio.com"}, nil, nil)
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
	err = json.Unmarshal(data, &ca2)
	is.NoErr(err)

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

	csr, err := ca2.CSR()
	is.NoErr(err)
	_, err = ca.Approve(csr)
	is.NoErr(err)
}
