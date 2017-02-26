package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"

	c4 "github.com/Avalanche-io/c4/id"
)

// An entity is a person identified by email or phone number, or a computer
// identified by an IP or MAC address.
type Entity struct {
	Ident     Identifier
	IdentType IdentifierType
	pri       *ecdsa.PrivateKey
	pub       *ecdsa.PublicKey
}

// An Identifier is a email, phone number, ip address, or MAC address used
// to identify and validate Entities.
type Identifier string

// An IdentifierType holds the kind of identifier being used.
type IdentifierType uint

const (
	EMail IdentifierType = iota
	PhoneNumber
	IP
	MAC
)

// Signature stores the signing information for a particular ID, and Entity.
type Signeture struct {
	id     *c4.ID
	entity *Entity
	r      *big.Int
	s      *big.Int
}

// NewEntity creates a new entity with new private and public key pair.
//
// Use this method only when needed to create an entirely new Entity.
//
// When creating an Entity for signature verification create an empty
// Entity and use SetPublic() to set the public key for that Entity.
func NewEntity(ident string, itype IdentifierType) (*Entity, error) {
	random := rand.Reader
	// bits := 2048
	pri, err := ecdsa.GenerateKey(elliptic.P521(), random)
	if err != nil {
		return nil, err
	}

	ent := Entity{Identifier(ident), itype, pri, &pri.PublicKey}
	return &ent, nil
}

// Public() returns the public key for the entity as a crypto.PublicKey interface.
func (e *Entity) Public() crypto.PublicKey {
	return e.pub
}

// Private return the private key for the entity.
func (e *Entity) Private() *ecdsa.PrivateKey {
	return e.pri
}

// Sign since a given ID with the Entity's private key.
func (e *Entity) Sign(id *c4.ID) (*Signeture, error) {
	r, s, err := ecdsa.Sign(rand.Reader, e.pri, id.RawBytes())
	if err != nil {
		return nil, err
	}
	return &Signeture{id, e, r, s}, nil
}

// SetPublic sets the public key of the Entity, and removes the private key (if any).
// After SetPublic the Entity can only be used for signature validation, not signing.
func (e *Entity) SetPublic(key crypto.PublicKey) {
	e.pri = nil
	e.pub = key.(*ecdsa.PublicKey)
}

// Verify returns true if the Entity is the signer, and false otherwise.
func (e *Entity) Verify(sig *Signeture) bool {
	if sig == nil || sig.id == nil {
		return false
	}
	return ecdsa.Verify(e.pub, sig.id.RawBytes(), sig.r, sig.s)
}

// ID returns the c4 id to which this signature applies.  If this returned ID
// is nil then the signature is not initialized.
func (s *Signeture) ID() *c4.ID {
	return s.id
}
