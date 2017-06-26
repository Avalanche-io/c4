package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	c4 "github.com/Avalanche-io/c4/id"
	c4time "github.com/Avalanche-io/c4/time"
)

// The User type represents a human Entity.
type User struct {
	Identities      []Identifier `json:"identities"`
	ClearPrivateKey *PrivateKey  `json:"-"`
	// EncryptedPrivateKey *pem.Block  `json:"encrypted_private_key"`
	EncryptedPrivateKey []byte     `json:"encrypted_private_key"`
	PublicKey           *PublicKey `json:"public_key"`
	Certificate         *Cert      `json:"certificate"`
	ClearPassphrase     []byte     `json:"-"`
	EncryptedPassphrase []byte     `json:"encrypted_passphrase"`
	Salt                []byte     `json"salt"`
}

// An Identifier is a email, phone number, ip address, or MAC address used
// to identify and validate Entities.
type Identifier struct {
	Name string
	Type uint
}

// An IdentifierType holds the kind of identifier being used.
const (
	EMail uint = iota
	PhoneNumber
	URL
	IP
	MAC
)

// UserKeyUsage specifies the x509 KeyUsage flags.
// These values are used for certificate creation.
const UserKeyUsage x509.KeyUsage = x509.KeyUsageDigitalSignature |
	x509.KeyUsageContentCommitment |
	x509.KeyUsageKeyEncipherment |
	x509.KeyUsageDataEncipherment |
	x509.KeyUsageKeyAgreement |
	x509.KeyUsageCertSign |
	x509.KeyUsageCRLSign

// UserExtUsage specifies an array of x509 ExtKeyUsage flags.
// These values are used for certificate creation.
var UserExtUsage []x509.ExtKeyUsage = []x509.ExtKeyUsage{
	x509.ExtKeyUsageAny,
}

// NewUser creates a user entity. Optional arguments can specify
// one or more token value pairs specifying how the user should
// be globally identified (usually by email address).
//
// Example of setting an email address:
// `user, err := pki.NewUser("john.doe@example.com", pki.EMail)`
//
func NewUser(identifiers ...interface{}) (*User, error) {
	idents := []Identifier{}
	var name string
	var t uint

	for i := 0; i < len(identifiers)-1; i++ {
		switch val := identifiers[i].(type) {
		case string:
			name = val
		default:
			return nil, ErrNewUser(i)
		}
		i++
		switch val := identifiers[i].(type) {
		case uint:
			t = val
		default:
			return nil, ErrNewUser(i)
		}
		idents = append(idents, Identifier{name, t})
	}
	if len(idents) < 1 {
		return nil, ErrNewUser(0)
	}
	b := make([]byte, 64)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	u := User{
		Identities: idents,
		Salt:       b,
	}
	return &u, nil
}

// Passphrase checks a passphrase, and will set the encrypted
// passphrase filed if it is nil.  If the encrypted passphrase
// filed is not empty and does not match an error is returned.
func (u *User) Passphrase(passphrase string) (err error) {
	u.ClearPassphrase = []byte(passphrase)

	// If the encrypted passphrase is empty, then encrypt the one provided
	if u.EncryptedPassphrase == nil {
		err = u.set_passphrase()
	}
	if err != nil {
		return err
	}

	// Check passphrase (even if we just created it)
	err = u.check_passphrase()
	if err != nil {
		return err
	}

	// Decrypt or encrypt private key as necessary
	u.manage_keys()
	return nil
}

func (e *User) Logout() {
	e.ClearPassphrase = nil
	e.ClearPrivateKey = nil
}

func (e *User) ChangePassphrase(oldpassphrase string, newpassphrase string) error {
	oldpw := e.EncryptedPassphrase
	oldpk := e.EncryptedPrivateKey
	if oldpw == nil {
		return ErrChangeNilPassphrase{}
	}
	err := e.Passphrase(oldpassphrase)
	if err != nil {
		return err
	}

	e.EncryptedPassphrase = nil
	e.EncryptedPrivateKey = nil
	err = e.Passphrase(newpassphrase)
	if err != nil {
		e.EncryptedPassphrase = oldpw
		e.EncryptedPrivateKey = oldpk
		return err
	}
	return nil
}

// ID is not yet implemented, but will return the unique identifier for the user.
func (e *User) ID() *c4.ID {
	return nil
}

// Name returns a comma separated list of names from the list of Identities this
// user has (i.e. all the email addresses, and phone numbers)
func (e *User) Name() string {
	var out []string
	for _, v := range e.Identities {
		out = append(out, v.Name)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// GenerateKeys generates new private and public key pairs. It overwrites
// any previous keys.
func (e *User) GenerateKeys() error {
	pri, _, err := generateKeys()
	if err != nil {
		return err
	}
	e.ClearPrivateKey = (*PrivateKey)(pri)
	return nil
}

// Public returns the users public key.
func (e *User) Public() *PublicKey {
	if e.ClearPrivateKey != nil {
		return e.ClearPrivateKey.Public()
	}
	return nil
}

// Private returns the users unencrypted private key, if it is available.
// Otherwise it returns nil.
func (e *User) Private() *PrivateKey {
	return e.ClearPrivateKey
}

// Sign the users signature of id.
func (e *User) Sign(id *c4.ID) (*Signature, error) {
	return NewSignature(e.ClearPrivateKey, id)
}

// SetCert assigned cert to the user. This only needs to be done when
// certificates that have been signed by a certificate authority.
func (e *User) SetCert(cert *Cert) {
	e.Certificate = cert
}

// Cert returns the users current certificate.
func (e *User) Cert() *Cert {
	return e.Certificate
}

// TLScert returns tls.Certificate for easy use with TLS connections
func (e *User) TLScert(t TLScertType) (tls.Certificate, error) {
	return tls.X509KeyPair(e.Certificate.PEM(), e.Private().PEM())
}

// Endorse creates a certificate for target signed by this user.
func (e *User) Endorse(target Entity) (*Cert, error) {
	t_pub := (*ecdsa.PublicKey)(target.Public())
	e_pri := (*ecdsa.PrivateKey)(e.Private())
	certDER, err := x509.CreateCertificate(rand.Reader, target.Cert().X509(), e.Cert().X509(), t_pub, e_pri)
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

// CSR creates a certificate signing request for use by this user
// the csr must then be presented to a certificate authority to be
// validated.
func (e *User) CSR() (*CertificateSigningRequest, error) {
	cn := e.Name()
	if len(cn) == 0 {
		return nil, ErrBadCommonName{}
	}

	name := pkix.Name{
		CommonName: cn,
	}
	var email []string
	var phone []string

	for _, ident := range e.Identities {
		switch ident.Type {
		case EMail:
			email = append(email, ident.Name)
		case PhoneNumber:
			phone = append(phone, ident.Name)
		}
	}

	tmpl := &x509.CertificateRequest{
		Subject:        name,
		EmailAddresses: email,
	}

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

func (e *User) Approve(csr *CertificateSigningRequest) (*Cert, error) {
	csr.Varify(e)
	req := csr.CR()
	err := req.CheckSignature()
	if err != nil {
		return nil, err
	}

	domain_type := true
	usage := UserKeyUsage
	ext := UserExtUsage
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

func (u *User) set_passphrase() error {
	cipertext, err := bcrypt.GenerateFromPassword(u.ClearPassphrase, 12)
	if err != nil {
		return err
	}
	u.EncryptedPassphrase = cipertext
	return nil
}

func (u *User) check_passphrase() error {
	err := bcrypt.CompareHashAndPassword(u.EncryptedPassphrase, u.ClearPassphrase)
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			err = ErrBadPassphrase{}
		}
	}
	return err
}

func (u *User) manage_keys() {
	if u.ClearPrivateKey == nil && u.EncryptedPrivateKey == nil {
		return
	}
	if u.ClearPrivateKey != nil && u.EncryptedPrivateKey != nil {
		return
	}

	if u.ClearPrivateKey != nil {
		u.encode_privatekey()
		return
	}

	u.decrypt_privatekey()
}

func (u *User) decrypt_privatekey() {
	blk, _ := pem.Decode(u.EncryptedPrivateKey)
	data := blk.Bytes
	if x509.IsEncryptedPEMBlock(blk) {
		key := append(u.Salt, u.ClearPassphrase...)
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
	u.ClearPrivateKey = (*PrivateKey)(pri)
}

// func (u *User) decrypt_privatekey() {
// 	key := append(u.Salt, u.ClearPassphrase...)
// 	data, err := x509.DecryptPEMBlock(u.EncryptedPrivateKey, key)
// 	if err != nil {
// 		return
// 	}
// 	k, err := x509.ParseECPrivateKey(data)
// 	if err != nil {
// 		return
// 	}
// 	u.ClearPrivateKey = (*PrivateKey)(k)
// }

func (u *User) encode_privatekey() {
	// Encode private key to pem block
	kb, err := x509.MarshalECPrivateKey((*ecdsa.PrivateKey)(u.ClearPrivateKey))
	if err != nil {
		return
	}
	blk := &pem.Block{Type: "PRIVATE KEY", Bytes: kb}

	// If there is a passphrase available then we encrypt the key
	if len(u.ClearPassphrase) > 0 {
		key := append(u.Salt, u.ClearPassphrase...)
		blk, err = x509.EncryptPEMBlock(rand.Reader, "ENCRYPTED PRIVATE KEY", kb, key, x509.PEMCipherAES256)
		if err != nil {
			return
		}
	}

	data := pem.EncodeToMemory(blk)
	u.EncryptedPrivateKey = data
}
