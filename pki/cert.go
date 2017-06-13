package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"time"

	c4time "github.com/Avalanche-io/c4/time"
)

type Cert x509.Certificate

// X509() *x509.Certificate
// PEM() []byte
// Verify(string) ([][]*x509.Certificate, error)

// An entity is a person identified by email or phone number, or a computer
// identified by an IP or MAC address.
// type standardCert
// type rootCert x509.Certificate

func (c *Cert) X509() *x509.Certificate {
	return (*x509.Certificate)(c)
}

func (c *Cert) PEM() []byte {
	b := pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}
	return pem.EncodeToMemory(&b)
}

func (c *Cert) Verify(name string) ([][]*x509.Certificate, error) {
	cert := (*x509.Certificate)(c)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	opts := x509.VerifyOptions{
		DNSName: name,
		Roots:   roots,
	}
	return cert.Verify(opts)
}

// func (c *rootCert) X509() *x509.Certificate {
// 	return (*x509.Certificate)(c)
// }

// func (c *rootCert) PEM() []byte {
// 	b := pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}
// 	return pem.EncodeToMemory(&b)
// }

// func (c *rootCert) Verify(name string) ([][]*x509.Certificate, error) {
// 	cert := (*x509.Certificate)(c)
// 	roots := x509.NewCertPool()
// 	roots.AddCert(cert)
// 	opts := x509.VerifyOptions{
// 		DNSName: name,
// 		Roots:   roots,
// 	}
// 	return cert.Verify(opts)
// }

// From RFC 5280, PKIX Certificate and CRL Profile, May 2008:
// (P 24)
//
// If subject naming information is present only in the subjectAltName extension
// (e.g., a key bound only to an email address or URI), then the subject name MUST
// be an empty sequence and the subjectAltName extension MUST be critical.

// endorse implements x509 certificate signing.
func endorse(e Entity, target Entity) (*Cert, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, errors.New("failed to generate serial number: " + err.Error())
	}
	// serialNumber * c4.ID
	now := c4time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          (*big.Int)(serialNumber),
		Subject:               pkix.Name{Organization: []string{"C4"}},
		SignatureAlgorithm:    x509.ECDSAWithSHA512,
		NotBefore:             now.AsTime(),
		NotAfter:              now.AsTime().Add(time.Hour * 24 * 30), // 1 month.
		BasicConstraintsValid: true,
	}
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}

	pri := (*ecdsa.PrivateKey)(e.Private())
	if pri == nil {
		panic("fuck off and die")
	}
	// create a certificate which wraps the targets public key, sign it with the root private key
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, e.Cert().X509(), (*ecdsa.PublicKey)(target.Public()), pri)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	target.SetCert((*Cert)(cert))

	return (*Cert)(cert), nil
}
