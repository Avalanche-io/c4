package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"math/big"

	c4 "github.com/Avalanche-io/c4/id"
)

func ParseCertificateRequest(der []byte) (*CertificateSigningRequest, error) {
	req, err := x509.ParseCertificateRequest(der)
	if err != nil {
		return nil, err
	}
	return &CertificateSigningRequest{der, req}, nil
}

type CertificateSigningRequest struct {
	der []byte
	cr  *x509.CertificateRequest
}

func (c *CertificateSigningRequest) inflate() error {
	switch {
	case c.cr == nil && c.der == nil:
		return errors.New("empty CertificateSigningRequest")
	case c.cr == nil && c.der != nil:
		cr, err := x509.ParseCertificateRequest(c.der)
		if err != nil {
			return err
		}
		c.cr = cr
	case c.cr != nil && c.der == nil:
		c.der = c.cr.RawTBSCertificateRequest
	case c.cr != nil && c.der != nil:

	}
	return nil
}

func (c *CertificateSigningRequest) Varify(e Entity) bool {
	err := c.inflate()
	if err != nil {
		return false
	}

	id := c4.Identify(bytes.NewReader(c.cr.RawTBSCertificateRequest))
	ecdsaSig := new(struct{ R, S *big.Int })
	_, err = asn1.Unmarshal(c.cr.Signature, ecdsaSig)
	if err != nil {
		return false
	}
	if e.Public() == nil {
		return false
	}
	return ecdsa.Verify((*ecdsa.PublicKey)(e.Public()), id.Digest(), ecdsaSig.R, ecdsaSig.S)
}

func (c *CertificateSigningRequest) Email() ([]string, error) {
	err := c.inflate()
	if err != nil {
		return nil, err
	}
	return c.cr.EmailAddresses, nil
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
