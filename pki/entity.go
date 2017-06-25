package pki

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sort"

	c4 "github.com/Avalanche-io/c4/id"
	"github.com/Avalanche-io/c4/time"
)

// An Entity is the generic security type for anything that can have a public
// private key pair, generally a person, company, or computr.
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

// An EntityBuilder provides a simplified API for setting the options
// needed to create a valid entity.
//
// Usage:
// builder := new(pki.EntityBuilder)
//
// // Required
// builder.AddDomain("example.com") // or AddEmail, AddIP, etc.
// new_entity := builder.Build()
// // Optional:
// // builder.DistinquisedName(pkix.Name)
// // builder.CommonName(value string)
// // builder.Organization(value string)
// // builder.OrganizationalUnit(value string)
// // builder.StreetAddress(value string)
// // builder.Locality(value string)
// // builder.Province(value string)
// // builder.PostalCode(value string)
// // builder.Country(value string)
// // builder.SetSerialNumber(*bit.Int)
// // builder.ValidBetween(start, end c4time)
// // builder.IsCA(flag bool)

const (
	notype int = iota
	domaintype
	usertype
)

type EntityBuilder struct {
	x509name   *pkix.Name
	name       string
	entityType int
	domains    []string
	ips        []string
	emails     []string
	passphrase string
	start      time.Time
	end        time.Time
	sn         *big.Int
	ca         bool
}

func (b *EntityBuilder) Domains(domains ...string) error {
	if len(domains) == 0 {
		return nil
	}
	if b.entityType == usertype {
		return errors.New("cannot add domains to a user Entity")
	}
	b.entityType = domaintype
	b.domains = append(b.domains, domains...)
	return nil
}

func (b *EntityBuilder) IPs(ips ...string) error {
	if len(ips) == 0 {
		return nil
	}
	if b.entityType == usertype {
		return errors.New("cannot add ip addresses to a user Entity")
	}
	b.entityType = domaintype
	b.ips = append(b.ips, ips...)
	return nil
}

func (b *EntityBuilder) Emails(emails ...string) error {
	if len(emails) == 0 {
		return nil
	}
	if b.entityType == domaintype {
		return errors.New("cannot add emails to a domain Entity")
	}
	b.entityType = usertype
	b.emails = append(b.emails, emails...)
	return nil
}

func (b *EntityBuilder) makename() {
	if b.x509name == nil {
		b.x509name = new(pkix.Name)
	}
}

func (b *EntityBuilder) Passphrase(passphrase string) {
	b.passphrase = passphrase
}

func (b *EntityBuilder) DistinquisedName(name pkix.Name) {
	b.x509name = &name
}

func (b *EntityBuilder) CommonName(name string) {
	b.makename()
	b.x509name.CommonName = name
}

func (b *EntityBuilder) Organizations(names ...string) {
	b.makename()
	b.x509name.Organization = append(b.x509name.Organization, names...)
}

func (b *EntityBuilder) OrganizationalUnits(names ...string) {
	b.makename()
	b.x509name.OrganizationalUnit = append(b.x509name.OrganizationalUnit, names...)
}

func (b *EntityBuilder) StreetAddress(addresses ...string) {
	b.makename()
	b.x509name.StreetAddress = append(b.x509name.StreetAddress, addresses...)
}

func (b *EntityBuilder) Localities(names ...string) {
	b.makename()
	b.x509name.Locality = append(b.x509name.Locality, names...)
}

func (b *EntityBuilder) Provinces(names ...string) {
	b.makename()
	b.x509name.Province = append(b.x509name.Province, names...)
}

func (b *EntityBuilder) PostalCodes(codes ...string) {
	b.makename()
	b.x509name.PostalCode = append(b.x509name.PostalCode, codes...)
}

func (b *EntityBuilder) Countries(names ...string) {
	b.makename()
	b.x509name.Country = append(b.x509name.Country, names...)
}

func (b *EntityBuilder) SetSerialNumber(number *big.Int) {
	b.makename()
	b.sn = number
}

func (b *EntityBuilder) ValidBetween(start, end time.Time) {
	b.start = start
	b.end = end
}

func (b *EntityBuilder) IsCA(flag bool) {
	b.ca = flag
}

func unique(list []string) []string {
	var out []string
	sort.Strings(list)
	previous := ""
	for _, name := range list {
		if name == previous {
			continue
		}
		previous = name
		out = append(out, name)
	}
	return out
}

func (b *EntityBuilder) buildDomain() (Entity, error) {
	domains := unique(b.domains)
	var ips []net.IP
	iplist := unique(b.ips)
	for _, ip := range iplist {
		netip := net.ParseIP(ip)
		if netip == nil {
			return nil, fmt.Errorf("invalid ip %q", ip)
		}
		ips = append(ips, netip)
	}
	salt := make([]byte, 64)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, err
	}

	e := Domain{
		Domains: domains,
		IPs:     ips,
		Salt:    salt,
	}
	e.GenerateKeys()
	e.Passphrase(b.passphrase)
	sn := b.sn
	if sn == nil {
		sn = defaultSNGenerator()
	}
	start := b.start
	if start.Nil() {
		start = time.Now()
	}
	end := b.end
	if end.Nil() {
		end = start.Add(time.Year)
	}
	key_usage := DomainKeyUsage
	if b.ca {
		key_usage = AthortyKeyUsage
	}

	tmpl := x509.Certificate{
		SignatureAlgorithm: x509.ECDSAWithSHA512,

		SerialNumber:          sn,
		Subject:               *b.x509name,
		NotBefore:             start.AsTime(),
		NotAfter:              end.AsTime(),
		BasicConstraintsValid: b.ca,
		IsCA:        b.ca,
		KeyUsage:    key_usage,
		DNSNames:    domains,
		IPAddresses: ips,
	}

	if b.ca {
		// Build and sign certificate
		certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, e.Public(), e.Private())
		if err != nil {
			return nil, err
		}
		// Extract certificate from the DER encoding
		cert, err := x509.ParseCertificate(certDER)
		if err != nil {
			return nil, err
		}
		e.Certificate = (*Cert)(cert)
	}

	return &e, nil
}

func (b *EntityBuilder) buildUser() (Entity, error) {
	emails := unique(b.emails)
	var ids []Identifier
	for _, email := range emails {
		ids = append(ids, Identifier{email, EMail})
	}
	salt := make([]byte, 64)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, err
	}
	e := User{
		Identities: ids,
		Salt:       salt,
	}
	e.GenerateKeys()
	e.Passphrase(b.passphrase)
	return &e, nil
}

func (b *EntityBuilder) Build() (Entity, error) {
	switch b.entityType {
	case usertype:
		return b.buildUser()
	case domaintype:
		return b.buildDomain()
	}
	return nil, errors.New("incomplete information to build entity, must have domain, ip or email")
}
