package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"math/big"

	c4 "github.com/Avalanche-io/c4/id"
)

// Signature stores the signing information for a particular ID, and Entity.
type Signature struct {

	// ID of signature
	id *c4.ID

	// ID of signed asset
	asset *c4.ID

	// reference to the entity that did the signing
	key *PublicKey

	// elliptic curve signature components
	r *big.Int
	s *big.Int
}

func NewSignature(key *PrivateKey, id *c4.ID) (*Signature, error) {
	r, s, err := ecdsa.Sign(rand.Reader, key.Ecdsa(), id.Digest())
	if err != nil {
		return nil, err
	}

	pub := PublicKey(key.PublicKey)
	return &Signature{
		asset: id,
		key:   &pub,
		r:     r,
		s:     s,
	}, nil
}

// ID returns the c4 id to which this signature applies.  If this returned ID
// is nil then the signature is not initialized.
func (s *Signature) ID() *c4.ID {
	return s.id
}

func (s *Signature) R() *big.Int {
	return s.r
}

func (s *Signature) S() *big.Int {
	return s.s
}
