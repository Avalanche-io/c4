package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	c4 "github.com/Avalanche-io/c4/id"
	c4time "github.com/Avalanche-io/c4/time"
)

const DomainKeyUsage x509.KeyUsage = x509.KeyUsageDigitalSignature |
	x509.KeyUsageKeyEncipherment |
	x509.KeyUsageDataEncipherment

var DomainExtUsage []x509.ExtKeyUsage = []x509.ExtKeyUsage{
	x509.ExtKeyUsageClientAuth,
	x509.ExtKeyUsageServerAuth,
}

// A Domain is hierarchical Entity that represents one or more organizational
// domains.
type Domain struct {
	Domains             []string    `json:"domains"`
	IPs                 []net.IP    `json:"ips"`
	ClearPrivateKey     *PrivateKey `json:"-"`
	EncryptedPrivateKey *pem.Block  `json:"encrypted_private_key"`
	Certificate         *Cert       `json:"certificate"`
	ClearPassphrase     []byte      `json:"-"`
	EncryptedPassphrase []byte      `json:"encrypted_passphrase"`
	Salt                []byte      `json"salt"`

	// pri       *PrivateKey
	// PublicKey *PublicKey
	// C         Cert
}

// User is an Entity that represents a human user.
// type User struct {
// 	Identities          []Identifier `json:"identities"`
// 	ClearPrivateKey     *PrivateKey  `json:"-"`
// 	EncryptedPrivateKey *pem.Block   `json:"encrypted_private_key"`
// 	Certificate         Cert         `json:"certificate"`
// 	ClearPassphrase     []byte       `json:"-"`
// 	EncryptedPassphrase []byte       `json:"encrypted_passphrase"`
// 	Salt                []byte       `json"salt"`
// }

func NewDomain() (*Domain, error) {
	d := Domain{}
	return &d, nil
}

func (e *Domain) AddDomains(names ...string) {
	e.Domains = names
}

func (e *Domain) AddIPs(ips ...net.IP) {
	e.IPs = ips
}

func (e *Domain) set_passphrase() error {
	cipertext, err := bcrypt.GenerateFromPassword(e.ClearPassphrase, 12)
	if err != nil {
		return err
	}
	e.EncryptedPassphrase = cipertext
	return nil
}

func (e *Domain) check_passphrase() error {
	err := bcrypt.CompareHashAndPassword(e.EncryptedPassphrase, e.ClearPassphrase)
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			err = ErrBadPassphrase{}
		}
	}
	return err
}

// Passphrase checks a passphrase, and will set the encrypted
// passphrase filed if it is nil.  If the encrypted passphrase
// filed is not empty and does not match an error is returned.
func (e *Domain) Passphrase(passphrase string) (err error) {
	e.ClearPassphrase = []byte(passphrase)

	// If the encrypted passphrase is empty, then encrypt the one provided
	if e.EncryptedPassphrase == nil {
		err = e.set_passphrase()
	}
	if err != nil {
		return err
	}

	// Check passphrase (even if we just created it)
	err = e.check_passphrase()
	if err != nil {
		return err
	}

	// Decrypt or encrypt private key as necessary
	e.manage_keys()
	return nil
}

func (e *Domain) decrypt_privatekey() {
	key := append(e.Salt, e.ClearPassphrase...)
	data, err := x509.DecryptPEMBlock(e.EncryptedPrivateKey, key)
	if err != nil {
		return
	}
	k, err := x509.ParseECPrivateKey(data)
	if err != nil {
		return
	}
	e.ClearPrivateKey = (*PrivateKey)(k)
}

func (e *Domain) encrypt_privatekey() {
	key := append(e.Salt, e.ClearPassphrase...)
	kb, err := x509.MarshalECPrivateKey((*ecdsa.PrivateKey)(e.ClearPrivateKey))
	if err != nil {
		return
	}
	blk, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", kb, key, x509.PEMCipherAES256)
	if err != nil {
		return
	}
	e.EncryptedPrivateKey = blk
}

func (e *Domain) manage_keys() {
	if e.ClearPrivateKey == nil && e.EncryptedPrivateKey == nil {
		return
	}
	if e.ClearPrivateKey != nil && e.EncryptedPrivateKey != nil {
		return
	}

	if e.ClearPrivateKey != nil {
		e.encrypt_privatekey()
		return
	}

	e.decrypt_privatekey()
}

func (e *Domain) ID() *c4.ID {
	return nil
}

func (e *Domain) Name() string {
	return strings.Join(e.Domains, ",")
}

func (e *Domain) GenerateKeys() error {
	pri, _, err := generateKeys()
	if err != nil {
		return err
	}
	e.ClearPrivateKey = (*PrivateKey)(pri)
	return nil
}

func (e *Domain) Private() *PrivateKey {
	return e.ClearPrivateKey
}

func (e *Domain) Public() *PublicKey {
	if e.ClearPrivateKey != nil {
		return e.ClearPrivateKey.Public()
	}
	return nil
}

func (e *Domain) SetCert(cert *Cert) {
	e.Certificate = cert
}

func (e *Domain) Cert() *Cert {
	return e.Certificate
}

func (e *Domain) Sign(id *c4.ID) (*Signature, error) {
	return NewSignature(e.ClearPrivateKey, id)
}

func (e *Domain) TLScert(t TLScertType) (tls.Certificate, error) {
	return tls.X509KeyPair(e.Certificate.PEM(), e.Private().PEM())
}

func (e *Domain) Endorse(target Entity) (*Cert, error) {
	return endorse(e, target)
}

func (e *Domain) CSR() (*CertificateSigningRequest, error) {
	if len(e.Domains) == 0 && len(e.IPs) == 0 {
		return nil, ErrNoValidCn{}
	}

	// var cn string
	// if len(e.IPs) > 0 {
	// 	cn = e.IPs[0].String()
	// }

	// if len(e.Domains) > 0 {
	// 	cn = e.Domains[0]
	// }
	//organizational_unit, organization, country string
	name := pkix.Name{
		CommonName: e.Name(),
		// Country:            []string{country},
		// Organization:       []string{organization},
		// OrganizationalUnit: []string{organizational_unit},
		// Locality:           nil,
		// Province:           nil,
		// StreetAddress:      nil,
		// PostalCode:         nil,
		// SerialNumber:       "",
	}
	tmpl := &x509.CertificateRequest{
		Subject:     name,
		IPAddresses: e.IPs,
		DNSNames:    e.Domains,
	}
	// rawSubj := name.ToRDNSequence()

	req, err := x509.CreateCertificateRequest(rand.Reader, tmpl, (*ecdsa.PrivateKey)(e.ClearPrivateKey))
	if err != nil {
		return nil, err
	}

	cr, err := x509.ParseCertificateRequest(req)
	if err != nil {
		return nil, err
	}

	csr := &CertificateSigningRequest{der: req, cr: cr}
	return csr, nil
}

func (e *Domain) Approve(csr *CertificateSigningRequest) (*Cert, error) {
	csr.Varify(e)
	req := csr.CR()
	err := req.CheckSignature()
	if err != nil {
		return nil, err
	}

	domain_type := true
	usage := DomainKeyUsage
	ext := DomainExtUsage
	if len(req.EmailAddresses) > 0 {
		domain_type = false
		usage = UserKeyUsage
		ext = UserExtUsage
	}

	now := c4time.Now()
	b := make([]byte, 64)
	_, err = rand.Read(b)
	if err != nil {
		return nil, err
	}
	var sn big.Int
	sn.SetBytes(b)
	tmpl := x509.Certificate{
		SerialNumber:          &sn,
		Subject:               req.Subject,
		SignatureAlgorithm:    x509.ECDSAWithSHA512,
		NotBefore:             now.AsTime(),
		NotAfter:              now.AsTime().Add(time.Hour * 24 * 7), // 1 week
		BasicConstraintsValid: true,
		KeyUsage:              usage,
		ExtKeyUsage:           ext,
		PublicKey:             req.PublicKey,
	}
	if domain_type {
		tmpl.DNSNames = req.DNSNames
		tmpl.IPAddresses = req.IPAddresses
	} else {
		tmpl.EmailAddresses = req.EmailAddresses
	}

	// create the signed cert for the public key provided
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, e.Cert().X509(), req.PublicKey, (*ecdsa.PrivateKey)(e.Private()))
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	// target.SetCert((*standardCert)(cert))

	return (*Cert)(cert), nil

}
