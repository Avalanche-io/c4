package pki

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"

	c4time "github.com/Avalanche-io/c4/time"
)

// AthortyKeyUsage is exported for special uses cases, but should not
// be changed without very good reason.
const AthortyKeyUsage x509.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign

// SerialNumberGenerator function type for user supplied serial number generator
type SerialNumberGenerator func() *big.Int

// CreateAthorty creates a Certificate Authority Entity for use with C4 PKI.
// The authority is valid for 365 days.
//
// If the optional SerialNumberGenerator (sn) argument is provided it
// will be used to supply the certificate SerialNumber value, if not
// the default sn generator will be used which creates a 128 bit random number
// from crypt/rand.
func CreateAthorty(name pkix.Name, domains []string, ips []net.IP, sn ...SerialNumberGenerator) (Entity, error) {
	// Get serial number generator, or use default
	genSN := defaultSNGenerator
	if len(sn) > 0 {
		genSN = sn[0]
	}

	// c4time enforces udt
	now := c4time.Now()

	// Certificate template
	tmpl := x509.Certificate{
		SerialNumber: genSN(),
		Subject:      name,

		// C4 always uses EC and SHA-512 will lead to future efficiencies
		SignatureAlgorithm: x509.ECDSAWithSHA512,

		// All CA and certs should be normalized to UDT (hence c4time)
		// Expect certs to be used on devices that change time zones
		NotBefore: now.AsTime(),
		NotAfter:  now.AsTime().Add(time.Hour * 24 * 365),

		BasicConstraintsValid: true,
		IsCA:        true,
		KeyUsage:    AthortyKeyUsage,
		DNSNames:    domains,
		IPAddresses: ips,
	}

	// Generate elliptic keys
	pri, pub, err := generateKeys()
	if err != nil {
		return nil, err
	}

	// Build and sign certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, pub, pri)
	if err != nil {
		return nil, err
	}

	// Extract certificate from the DER encoding
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	// Build and return domain structure.
	entity := Domain{
		name:            name.CommonName,
		Domains:         domains,
		IPs:             ips,
		ClearPrivateKey: (*PrivateKey)(pri),
		Certificate:     (*Cert)(cert),
	}
	entity.encrypt_privatekey()
	return &entity, nil

}

func defaultSNGenerator() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil
	}
	return serialNumber
}
