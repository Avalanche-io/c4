package pki_test

import (
	"encoding/json"
	"testing"

	"github.com/cheekybits/is"

	"github.com/Avalanche-io/c4/pki"
)

func TestUserSaveLoad(t *testing.T) {
	is := is.New(t)
	u1, err := pki.NewUser("john.doe@example.com", pki.EMail)
	is.NoErr(err)
	is.NotNil(u1)
	is.NoErr(u1.GenerateKeys())

	is.NoErr(u1.Passphrase("some password"))

	// Save
	data, err := json.Marshal(u1)
	is.NoErr(err)

	// Load
	var u2 pki.User
	is.NoErr(json.Unmarshal(data, &u2))

	// Does not save the PrivateKey, or Passphrase in the clear
	is.NotNil(u2.EncryptedPrivateKey)
	is.Nil(u2.ClearPrivateKey)
	is.NotNil(u2.EncryptedPassphrase)
	is.Nil(u2.ClearPassphrase)

	is.NoErr(u2.Passphrase("some password"))
	is.NotNil(u2.Private())

	var u3 pki.User
	is.NoErr(json.Unmarshal(data, &u3))

	err = u3.Passphrase("wrong password")
	is.Err(err)
	is.Equal(err.Error(), "incorrect password")

	is.Nil(u3.Private())
}
