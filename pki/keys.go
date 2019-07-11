package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"

	c4 "github.com/Avalanche-io/c4/id"
)

// updated, delete me

// Public and Private Key interfaces define the most useful basic
// methods of cryptographic keys, for easier and safer interaction.
type PublicKey ecdsa.PublicKey

// Public and Private Key interfaces define the most useful basic
// methods of cryptographic keys, for easier and safer interaction.
type PrivateKey ecdsa.PrivateKey

func (k *PublicKey) MarshalJSON() ([]byte, error) {
	var ek ecdsa.PublicKey
	ek = ecdsa.PublicKey(*k)
	return json.Marshal(ek)
}

func (k *PublicKey) UnmarshalJSON(data []byte) error {
	var ek ecdsa.PublicKey
	ek.Curve = elliptic.P521()
	err := json.Unmarshal(data, &ek)
	if err != nil {
		return err
	}
	*k = PublicKey(ek)
	return nil
}

func (k *PublicKey) ID() *c4.ID {
	return nil
}

func (k *PublicKey) PEM() []byte {
	key := (*ecdsa.PublicKey)(k)
	// pem.Encode(out, b)
	keybytes, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{
		Type: "EC PUBLIC KEY", Bytes: keybytes,
	})
}

func (k *PublicKey) Ecdsa() *ecdsa.PublicKey {
	return (*ecdsa.PublicKey)(k)
}

func (k *PublicKey) Varify(s *Signature) bool {
	key := (*ecdsa.PublicKey)(k)
	return ecdsa.Verify(key, s.asset.Digest(), s.r, s.s)
}

func (k *PrivateKey) ID() *c4.ID {
	return nil
}

func (k *PrivateKey) PEM() []byte {
	key := (*ecdsa.PrivateKey)(k)
	keybytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{
		Type: "EC PRIVATE KEY", Bytes: keybytes,
	})
}

func (k *PrivateKey) Public() *PublicKey {
	key := (*ecdsa.PrivateKey)(k).PublicKey

	return (*PublicKey)(&key)
}

func (k *PrivateKey) Ecdsa() *ecdsa.PrivateKey {
	return (*ecdsa.PrivateKey)(k)
}

// Generate new elliptic curve dsa keys.
func generateKeys() (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	pri, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return pri, &pri.PublicKey, nil
}
