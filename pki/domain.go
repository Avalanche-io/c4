package pki

import (
	"crypto/tls"
	"strings"

	c4 "github.com/Avalanche-io/c4/id"
)

// A Domain is hierarchical Entity that represents one or more organizational
// domains.
type Domain struct {
	Names []string
	pri   *PrivateKey
	pub   *PublicKey
	cert  Cert
}

func NewDomain(domainNames ...string) (*Domain, error) {
	d := Domain{
		Names: domainNames,
	}
	return &d, nil
}

func (e *Domain) ID() *c4.ID {
	return nil
}

func (e *Domain) Name() string {
	return strings.Join(e.Names, ",")
}

func (e *Domain) GenerateKeys() error {
	pri, pub, err := generateKeys()
	if err != nil {
		return err
	}
	e.pri = (*PrivateKey)(pri)
	e.pub = (*PublicKey)(pub)
	return nil
}

func (e *Domain) Public() *PublicKey {
	return e.pub
}

func (e *Domain) Private() *PrivateKey {
	return e.pri
}

func (e *Domain) SetPublic(key *PublicKey) {
	e.pub = key
}

func (e *Domain) Sign(id *c4.ID) (*Signature, error) {
	return NewSignature(e.pri, id)
}

func (e *Domain) Verify(sig *Signature) bool {
	return sig.Varify(e)
}

func (e *Domain) TLScert(t TLScertType) (tls.Certificate, error) {
	return tls.X509KeyPair(e.Cert().PEM(), e.Private().PEM())
}

func (e *Domain) Endorse(target Entity) (Cert, error) {
	return endorse(e, target)
}

func (e *Domain) SetCert(c Cert) {
	e.cert = c
}

func (e *Domain) Cert() Cert {
	return e.cert
}

// func (e *Domain) MakeRootCert() (Cert, error) {
// 	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
// 	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
// 	if err != nil {
// 		return nil, errors.New("failed to generate serial number: " + err.Error())
// 	}
// 	now := c4time.Now()
// 	tmpl := x509.Certificate{
// 		SerialNumber:          (*big.Int)(serialNumber),
// 		Subject:               pkix.Name{Organization: []string{"C4 Root"}},
// 		SignatureAlgorithm:    x509.ECDSAWithSHA512,
// 		NotBefore:             now.AsTime(),
// 		NotAfter:              now.AsTime().Add(time.Hour * 24 * 30), // 1 month.
// 		BasicConstraintsValid: true,
// 	}
// 	// describe what the certificate will be used for
// 	tmpl.IsCA = true
// 	tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
// 	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
// 	// tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
// 	// tmpl.DNSNames = e.Domains

// 	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, e.Public(), e.Private())
// 	if err != nil {
// 		return nil, err
// 	}
// 	// parse the resulting certificate so we can use it again
// 	cert, err := x509.ParseCertificate(certDER)
// 	if err != nil {
// 		return nil, err
// 	}
// 	e.cert = (*standardCert)(cert)

// 	return e.cert, nil
// }
