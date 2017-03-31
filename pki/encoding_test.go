package pki

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"

	"testing"

	"github.com/cheekybits/is"
)

func TestEncodingDER(t *testing.T) {
	is := is.New(t)
	pri, _, err := generateKeys()
	is.NoErr(err)
	data, err := x509.MarshalECPrivateKey(pri)
	is.NoErr(err)
	pri2, err := x509.ParseECPrivateKey(data)
	is.NoErr(err)
	is.Equal(pri, pri2)
}

func TestEncodingPEM(t *testing.T) {
	is := is.New(t)
	pri, _, err := generateKeys()
	is.NoErr(err)
	data, err := x509.MarshalECPrivateKey(pri)
	is.NoErr(err)
	blk := pem.Block{Type: "PRIVATE KEY", Bytes: data}
	pembytes := pem.EncodeToMemory(&blk)
	is.NoErr(err)

	is.False(x509.IsEncryptedPEMBlock(&blk))

	// reverse
	blk2, rest := pem.Decode(pembytes)
	is.NotNil(blk2)
	is.Equal(len(rest), 0)
	pri2, err := x509.ParseECPrivateKey(blk2.Bytes)
	is.NoErr(err)
	is.Equal(pri2, pri)
}

func TestEncryption(t *testing.T) {
	is := is.New(t)
	pri, _, err := generateKeys()
	is.NoErr(err)
	data, err := x509.MarshalECPrivateKey(pri)
	is.NoErr(err)
	blk, err := x509.EncryptPEMBlock(rand.Reader, "ENCRYPTED PRIVATE KEY", data, []byte("password"), x509.PEMCipherAES256)
	is.NoErr(err)
	pembytes := pem.EncodeToMemory(blk)
	is.NoErr(err)

	is.True(x509.IsEncryptedPEMBlock(blk))

	// reverse
	blk2, rest := pem.Decode(pembytes)
	is.NotNil(blk2)
	is.Equal(len(rest), 0)
	data2, err := x509.DecryptPEMBlock(blk2, []byte("password"))
	is.NoErr(err)
	pri2, err := x509.ParseECPrivateKey(data2)
	is.NoErr(err)
	is.Equal(pri2, pri)

	// wrong password
	data3, err := x509.DecryptPEMBlock(blk2, []byte("wrong password"))
	is.Err(err)
	pri3, err := x509.ParseECPrivateKey(data3)
	is.Err(err)
	is.NotEqual(pri3, pri)
}
