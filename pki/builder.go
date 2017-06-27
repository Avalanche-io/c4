package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sort"
	"time"

	c4time "github.com/Avalanche-io/c4/time"
)

const (
	notype int = iota
	domaintype
	usertype
)

// An Builder provides a simplified API for setting the options
// needed to create a valid entity.
//
// Usage:
// builder := new(pki.Builder)
//
// // Required
// builder.AddDomain("example.com") // or AddEmail, AddIP, etc.
// new_entity := builder.Build()
// // Either DistinquisedName, or CommonName must be called
// builder.DistinquisedName(pkix.Name)
// // builder.CommonName(value string)
// //
// // The rest are optional
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

type Builder struct {
	messages   chan string
	errors     chan error
	x509name   *pkix.Name
	name       string
	entityType int
	domains    []string
	ips        []net.IP
	emails     []string
	passphrase string
	start      c4time.Time
	end        c4time.Time
	sn         *big.Int
	salt       []byte
	ca         bool
}

// NewBuilder creates a new builder object.
func NewBuilder(name string) *Builder {
	b := new(Builder)
	b.messages = make(chan string, 128)
	b.errors = make(chan error, 128)
	b.salt = make([]byte, 64)
	_, err := rand.Read(b.salt)
	if err != nil {
		b.errors <- err
	}

	return b
}

// Listen returns a channels of status messages that provides
// progress information, for debugging and user feedback. The channel is closed
// at the end of the .build() method.
func (b *Builder) Listen() chan string {
	return b.messages
}

// Errors returns a channel of errors so that more than one error can be
// can be presented to the user at a time. Builder will attempt to continue
// after errors, so multiple errors can be detected and addressed at once
// by users.
func (b *Builder) Errors() chan error {
	return b.errors
}

// Domains takes a parameter list of zero or more domains, all of which will
// be added to the entity and certificate. If zero domains are given, the
// methods returns immediately with no effect or error.
func (b *Builder) Domains(domains ...string) {
	if len(domains) == 0 {
		b.messages <- "Zero domains added."
		return
	}
	if b.entityType == usertype {
		b.errors <- errors.New("added domains to a user Entity")
	}
	b.entityType = domaintype
	b.domains = append(b.domains, domains...)
	b.messages <- fmt.Sprintf("%d domains added.", len(domains))
	return
}

func (b *Builder) IPs(ips ...string) {
	if len(ips) == 0 {
		b.messages <- "Zero ips added."
		return
	}
	if b.entityType == usertype {
		b.errors <- errors.New("added ip addresses to a user Entity")
	}
	b.entityType = domaintype
	iplist := unique(ips)
	for _, ip := range iplist {
		netip := net.ParseIP(ip)
		if netip == nil {
			b.errors <- fmt.Errorf("invalid ip %q, ignored", ip)
		}
		b.ips = append(b.ips, netip)
	}
	b.messages <- fmt.Sprintf("%d ips added.", len(ips))
	return
}

func (b *Builder) Emails(emails ...string) {
	if len(emails) == 0 {
		b.messages <- "Zero email addresses added."
		return
	}
	if b.entityType == domaintype {
		b.errors <- errors.New("cannot add emails to a domain Entity")
	}
	b.entityType = usertype
	b.emails = append(b.emails, emails...)
	b.messages <- fmt.Sprintf("%d email addresses added.", len(emails))
	return
}

func makename(name *pkix.Name) *pkix.Name {
	if name == nil {
		return new(pkix.Name)
	}
	return name
}

func (b *Builder) Passphrase(passphrase string) {
	if len(passphrase) == 0 {
		return
	}
	b.passphrase = passphrase
}

func (b *Builder) DistinquisedName(name pkix.Name) {
	b.x509name = &name
	b.messages <- "Added distinquised name"
}

func (b *Builder) CommonName(name string) {
	b.x509name = makename(b.x509name)
	b.x509name.CommonName = name
	b.messages <- "Added CommonName name"
}

func (b *Builder) Organizations(names ...string) {
	b.x509name = makename(b.x509name)
	b.x509name.Organization = append(b.x509name.Organization, names...)
	// messages <- fmt.Sprintf(" %d Organizations", len(b.x509name.Organization))
}

func (b *Builder) OrganizationalUnits(names ...string) {
	b.x509name = makename(b.x509name)
	b.x509name.OrganizationalUnit = append(b.x509name.OrganizationalUnit, names...)
	// messages <- fmt.Sprintf(" %d OrganizationalUnits", len(b.x509name.OrganizationalUnit))
}

func (b *Builder) StreetAddress(addresses ...string) {
	b.x509name = makename(b.x509name)
	b.x509name.StreetAddress = append(b.x509name.StreetAddress, addresses...)
	// messages <- fmt.Sprintf(" %d StreetAddress", len(b.x509name.StreetAddress))
}

func (b *Builder) Localities(names ...string) {
	b.x509name = makename(b.x509name)
	b.x509name.Locality = append(b.x509name.Locality, names...)
	// messages <- fmt.Sprintf(" %d Localities", len(b.x509name.Localities))
}

func (b *Builder) Provinces(names ...string) {
	b.x509name = makename(b.x509name)
	b.x509name.Province = append(b.x509name.Province, names...)
	// messages <- fmt.Sprintf(" %d Provinces", len(b.x509name.Localities))
}

func (b *Builder) PostalCodes(codes ...string) {
	b.x509name = makename(b.x509name)
	b.x509name.PostalCode = append(b.x509name.PostalCode, codes...)
	// messages <- fmt.Sprintf(" %d PostalCodes", len(b.x509name.PostalCode))
}

func (b *Builder) Countries(names ...string) {
	b.x509name = makename(b.x509name)
	b.x509name.Country = append(b.x509name.Country, names...)
	b.messages <- fmt.Sprintf(" %d Countries", len(b.x509name.Country))
}

func (b *Builder) SetSerialNumber(number *big.Int) {
	b.x509name = makename(b.x509name)
	if number == nil && b.sn == nil {
		b.messages <- "Generating Random SerialNumber."
		b.sn = defaultSNGenerator()
	}
	b.sn = number
	b.messages <- "Set assigned SerialNumber."
}

func (b *Builder) Duration(days int) {
	b.start = c4time.Now()
	b.end = b.start.Add(c4time.Day * time.Duration(days))
	s := b.start.AsTime().Format("1/2/2006")
	e := b.end.AsTime().Format("1/2/2006")
	b.messages <- fmt.Sprintf("Valid duration %d. From %s to %s.", days, s, e)
}

func (b *Builder) ValidBetween(start, end c4time.Time) {
	b.start = start
	if b.start.Nil() {
		b.start = c4time.Now()
	}
	b.end = end
	if b.end.Nil() {
		b.end = b.start.Add(c4time.Year)
	}
	s := b.start.AsTime().Format("1/2/2006")
	e := b.end.AsTime().Format("1/2/2006")
	b.messages <- fmt.Sprintf("From %s to %s.", s, e)
}

func (b *Builder) IsCA(flag bool) {
	if flag {
		b.messages <- "Enabling Certificate Authority features."
	}

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

func (b *Builder) buildDomain() *Domain {
	if b.start.Nil() || b.end.Nil() || b.end.Sub(b.start) < c4time.Day {
		// create default time range
		b.start = c4time.Now()
		b.end = b.start.Add(c4time.Day * 365)
	}
	domains := unique(b.domains)

	e := Domain{
		Domains: domains,
		IPs:     b.ips,
		Salt:    b.salt,
	}
	if len(b.passphrase) == 0 {
		b.errors <- errors.New("Warning passphrase not set")
	} else {
		e.Passphrase(b.passphrase)
	}
	b.messages <- "Generating Keys"
	e.GenerateKeys()

	sn := b.sn
	m := "Setting "
	if sn == nil {
		m += "New "
		sn = defaultSNGenerator()
	}
	b.messages <- m + "SerialNumber"
	key_usage := DomainKeyUsage
	if b.ca {
		b.messages <- "Creating cert with AthortyKeyUsage"
		key_usage = AthortyKeyUsage
	}

	tmpl := x509.Certificate{
		SignatureAlgorithm: x509.ECDSAWithSHA512,

		SerialNumber:          sn,
		Subject:               *b.x509name,
		NotBefore:             b.start.AsTime(),
		NotAfter:              b.end.AsTime(),
		BasicConstraintsValid: b.ca,
		IsCA:        b.ca,
		KeyUsage:    key_usage,
		DNSNames:    domains,
		IPAddresses: b.ips,
	}

	// Build and sign certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, (*ecdsa.PublicKey)(e.Public()), (*ecdsa.PrivateKey)(e.Private()))
	if err != nil {
		b.errors <- err
	}
	b.messages <- "Certificate created."
	// Extract certificate from the DER encoding
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		b.errors <- err
	}
	e.Certificate = (*Cert)(cert)

	return &e
}

func (b *Builder) buildUser() *User {
	emails := unique(b.emails)
	var ids []Identifier
	for _, email := range emails {
		ids = append(ids, Identifier{email, EMail})
	}
	e := User{
		Identities: ids,
		Salt:       b.salt,
	}
	e.GenerateKeys()

	sn := b.sn
	if sn == nil {
		sn = defaultSNGenerator()
	}

	key_usage := DomainKeyUsage
	if b.ca {
		key_usage = AthortyKeyUsage
	}

	tmpl := x509.Certificate{
		SignatureAlgorithm: x509.ECDSAWithSHA512,

		SerialNumber:          sn,
		Subject:               *b.x509name,
		NotBefore:             b.start.AsTime(),
		NotAfter:              b.end.AsTime(),
		BasicConstraintsValid: b.ca,
		IsCA:        b.ca,
		KeyUsage:    key_usage,
		DNSNames:    b.domains,
		IPAddresses: b.ips,
	}

	// Build and sign certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, (*ecdsa.PublicKey)(e.Public()), (*ecdsa.PrivateKey)(e.Private()))
	if err != nil {
		b.errors <- err
	}
	// Extract certificate from the DER encoding
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		b.errors <- err
	}
	e.Certificate = (*Cert)(cert)
	if len(b.passphrase) == 0 {
		b.errors <- errors.New("passphrase not set")
	} else {
		b.messages <- "Setting phrase on entity."
		e.Passphrase(b.passphrase)
	}

	return &e
}

func (b *Builder) Build() Entity {
	defer func() {
		close(b.errors)
		b.errors = nil
		close(b.messages)
		b.messages = nil
	}()
	switch b.entityType {
	case usertype:
		e := b.buildUser()
		if e != nil {
			b.messages <- "Entity created."
		}
		return e
	case domaintype:
		e := b.buildDomain()
		if e != nil {
			b.messages <- "Entity created."
		}
		return e
	}
	b.errors <- errors.New("incomplete information to build entity, must have domain, ip or email")
	return nil
}
