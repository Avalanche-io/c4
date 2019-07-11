package pki

import (
	"crypto/tls"

	c4 "github.com/Avalanche-io/c4/id"
)

// updated, delete me

// An Entity is the generic security type for anything that can have a public
// private key pair, generally a person, company, or computer.
type Entity interface {

	// Identification
	ID() *c4.ID
	Name() string

	// Keys
	GenerateKeys() error
	Private() *PrivateKey
	Public() *PublicKey

	// Signatures
	Sign(id *c4.ID) (*Signature, error)

	// TLS
	TLScert(t TLScertType) (tls.Certificate, error)

	// Certificates
	Endorse(e Entity) (*Cert, error)
	Cert() *Cert
	SetCert(*Cert)
	Approve(csr *CertificateSigningRequest) (*Cert, error)

	// Encryption
	Passphrase(passphrase string) error
}
