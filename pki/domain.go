package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	// "fmt"
	// "encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"golang.org/x/crypto/bcrypt"

	c4 "github.com/Avalanche-io/c4/id"
	c4time "github.com/Avalanche-io/c4/time"
)

const DomainKeyUsage x509.KeyUsage = x509.KeyUsageDigitalSignature |
	x509.KeyUsageKeyEncipherment |
	x509.KeyUsageDataEncipherment |
	x509.KeyUsageCertSign |
	x509.KeyUsageCRLSign

var DomainExtUsage []x509.ExtKeyUsage = []x509.ExtKeyUsage{
	x509.ExtKeyUsageClientAuth,
	x509.ExtKeyUsageServerAuth,
}

// A Domain is hierarchical Entity that represents one or more organizational
// domains.
type Domain struct {
	name            string
	Domains         []string    `json:"domains"`
	IPs             []net.IP    `json:"ips"`
	ClearPrivateKey *PrivateKey `json:"-"`
	// EncryptedPrivateKey *pem.Block  `json:"encrypted_private_key"`
	EncryptedPrivateKey []byte `json:"encrypted_private_key"`
	Certificate         *Cert  `json:"certificate"`
	ClearPassphrase     []byte `json:"-"`
	EncryptedPassphrase []byte `json:"encrypted_passphrase"`
	Salt                []byte `json:"salt"`
}

// NewDomain creates a domain entity.
func NewDomain(name string) (*Domain, error) {
	d := Domain{name: name}
	return &d, nil
}

// AddDomains adds domain names to a list of domain names this
// domain represents.
func (e *Domain) AddDomains(names ...string) {
	e.Domains = names
}

// AddIPs add ip addresses to the list of IPs this domain represents.
func (e *Domain) AddIPs(ips ...net.IP) {
	e.IPs = ips
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

// ID is not yet implemented, but will return the unique identifier for the domain.
func (e *Domain) ID() *c4.ID {
	return nil
}

// Name returns a comma separated list of domain names for this domain.
func (e *Domain) Name() string {
	return e.name
}

// GenerateKeys generates a new private/public key pair.
func (e *Domain) GenerateKeys() error {
	pri, _, err := generateKeys()
	if err != nil {
		return err
	}
	e.ClearPrivateKey = (*PrivateKey)(pri)
	e.encode_privatekey()
	return nil
}

// Private returns the unencrypted private key for the domain if accessible,
// and returns nil otherwise.
func (e *Domain) Private() *PrivateKey {
	if e.ClearPrivateKey == nil {
		e.decrypt_privatekey()
	}
	return e.ClearPrivateKey
}

// Public returns the public key for the domain.
func (e *Domain) Public() *PublicKey {
	if e.ClearPrivateKey != nil {
		return e.ClearPrivateKey.Public()
	}
	return nil
}

// SetCert replaces the domains cert with the one provided. This is only needed
// when receiving a singed certificate from a remote certificate authority.
func (e *Domain) SetCert(cert *Cert) {
	e.Certificate = cert
}

// Cert returns the domains certificate.
func (e *Domain) Cert() *Cert {
	return e.Certificate
}

// Sign returns a signature of id for this domain.
func (e *Domain) Sign(id *c4.ID) (*Signature, error) {
	return NewSignature(e.ClearPrivateKey, id)
}

// TLScert returns tls formatted certificates for easy use with TLS connections.
func (e *Domain) TLScert(t TLScertType) (tls.Certificate, error) {
	return tls.X509KeyPair(e.Certificate.PEM(), e.Private().PEM())
}

// Endorse creates a certificate for target signed by the domain.
func (e *Domain) Endorse(target Entity) (*Cert, error) {
	return endorse(e, target)
}

// func (e *Domain) MarshalJSON() ([]byte, error) {
// 	if e.EncryptedPassphrase != nil && len(e.EncryptedPassphrase) >= 0 {
// 		e.ClearPrivateKey = nil
// 	}
// 	return json.Marshal(e)
// }

// CSR generates a certificate signing request for the domain sutable for submission
// to a remote certificate authority for validation and signature.
func (e *Domain) CSR(names ...pkix.Name) (*CertificateSigningRequest, error) {
	var name pkix.Name
	if len(names) == 0 {
		e.AddDomains(e.name)
		name = pkix.Name{
			CommonName: e.name,
		}
	} else {
		name = names[0]
	}

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
	// name := pkix.Name{
	// CommonName: e.name,
	// Country:            []string{country},
	// Organization:       []string{organization},
	// OrganizationalUnit: []string{organizational_unit},
	// Locality:           nil,
	// Province:           nil,
	// StreetAddress:      nil,
	// PostalCode:         nil,
	// SerialNumber:       "",
	// }
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

// Approve creates a signed certificate form the certificate signing request provided.
// Currently certificates are hard coded to be valued for only one week at a time.
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
		NotAfter:              now.AsTime().Add(time.Hour * 24 * 30), // 1 week
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
	pri := (*ecdsa.PrivateKey)(e.ClearPrivateKey)
	if pri == nil {
		return nil, errors.New("invalid signing entity")
	}
	// create the signed cert for the public key provided
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, e.Cert().X509(), req.PublicKey, pri)
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

func (e *Domain) decrypt_privatekey() {
	blk, _ := pem.Decode(e.EncryptedPrivateKey)
	data := blk.Bytes
	if x509.IsEncryptedPEMBlock(blk) {
		key := append(e.Salt, e.ClearPassphrase...)
		var err error
		data, err = x509.DecryptPEMBlock(blk, key)
		if err != nil {
			return
		}
	}
	pri, err := x509.ParseECPrivateKey(data)
	if err != nil {
		return
	}
	e.ClearPrivateKey = (*PrivateKey)(pri)
}

func (e *Domain) encode_privatekey() {
	// Encode private key to pem block
	data, err := x509.MarshalECPrivateKey((*ecdsa.PrivateKey)(e.ClearPrivateKey))
	if err != nil {
		return
	}
	blk := &pem.Block{Type: "PRIVATE KEY", Bytes: data}

	// If there is a passphrase available then we encrypt the key
	if len(e.ClearPassphrase) > 0 {
		key := append(e.Salt, e.ClearPassphrase...)
		blk, err = x509.EncryptPEMBlock(rand.Reader, "ENCRYPTED PRIVATE KEY", data, key, x509.PEMCipherAES256)
		if err != nil {
			return
		}
	}

	data = pem.EncodeToMemory(blk)
	e.EncryptedPrivateKey = data
}

func (e *Domain) manage_keys() {
	if e.ClearPrivateKey == nil && e.EncryptedPrivateKey == nil {
		return
	}
	if e.ClearPrivateKey != nil && e.EncryptedPrivateKey != nil {
		return
	}

	if e.ClearPrivateKey != nil {
		e.encode_privatekey()
		return
	}

	e.decrypt_privatekey()
}
