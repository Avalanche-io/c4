package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
	"net"
	"strings"

	c4 "github.com/Avalanche-io/c4/id"
)

// A Domain is hierarchical Entity that represents one or more organizational
// domains.
type Domain struct {
	Domains   []string
	IPs       []net.IP
	pri       *PrivateKey
	PublicKey *PublicKey
	C         Cert
}

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

func (e *Domain) ID() *c4.ID {
	return nil
}

func (e *Domain) Name() string {
	return strings.Join(e.Domains, ",")
}

func (e *Domain) GenerateKeys() error {
	pri, pub, err := generateKeys()
	if err != nil {
		return err
	}
	e.pri = (*PrivateKey)(pri)
	e.PublicKey = (*PublicKey)(pub)
	return nil
}

func (e *Domain) Private() *PrivateKey {
	return e.pri
}

func (e *Domain) Public() *PublicKey {
	return e.PublicKey
}

func (e *Domain) SetCert(cert Cert) {
	e.C = cert
}

func (e *Domain) Cert() Cert {
	return e.C
}

func (e *Domain) Sign(id *c4.ID) (*Signature, error) {
	return NewSignature(e.pri, id)
}

func (e *Domain) TLScert(t TLScertType) (tls.Certificate, error) {
	return tls.X509KeyPair(e.C.PEM(), e.Private().PEM())
}

func (e *Domain) Endorse(target Entity) (Cert, error) {
	return endorse(e, target)
}

type CertificateSigningRequest struct {
	der []byte
	cr  *x509.CertificateRequest
}

func ParseCertificateRequest(der []byte) (*CertificateSigningRequest, error) {
	req, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return nil, err
	}
	return &CertificateSigningRequest{der, req}, nil
}

func (e *Domain) CSR(organizational_unit, organization, country string) (*CertificateSigningRequest, error) {
	if len(e.Domains) == 0 && len(e.IPs) == 0 {
		return nil, ErrNoValidCn{}
	}

	var cn string
	if len(e.IPs) > 0 {
		cn = e.IPs[0].String()
	}

	if len(e.Domains) > 0 {
		cn = e.Domains[0]
	}

	name := pkix.Name{
		CommonName:         cn,
		Country:            []string{country},
		Organization:       []string{organization},
		OrganizationalUnit: []string{organizational_unit},
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

	req, err := x509.CreateCertificateRequest(rand.Reader, tmpl, (*ecdsa.PrivateKey)(e.pri))
	if err != nil {
		fmt.Printf("here:\n\r%v, %v\n", tmpl, e.pri)
		return nil, err
	}

	cr, err := x509.ParseCertificateRequest(req)
	if err != nil {
		return nil, err
	}
	csr := &CertificateSigningRequest{der: req, cr: cr}
	return csr, nil
}

func (c *CertificateSigningRequest) Varify(e Entity) bool {
	id := c4.Identify(bytes.NewReader(c.cr.RawTBSCertificateRequest))
	ecdsaSig := new(struct{ R, S *big.Int })
	_, err := asn1.Unmarshal(c.cr.Signature, ecdsaSig)
	if err != nil {
		return false
	}
	return ecdsa.Verify((*ecdsa.PublicKey)(e.Public()), id.Digest(), ecdsaSig.R, ecdsaSig.S)
}

func (c *CertificateSigningRequest) ID() *c4.ID {
	return c4.Identify(bytes.NewReader(c.cr.RawTBSCertificateRequest))
}

func (c *CertificateSigningRequest) DER() []byte {
	return c.der
}

func (c *CertificateSigningRequest) CR() *x509.CertificateRequest {
	return c.cr
}
