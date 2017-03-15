package pki

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"golang.org/x/crypto/bcrypt"
	"strings"

	c4 "github.com/Avalanche-io/c4/id"
)

// User is an Entity that represents a a human user.
type User struct {
	Identities          []*Identifier `json:"identities"`
	ClearPrivateKey     *PrivateKey   `json:"-"`
	EncryptedPrivateKey *pem.Block    `json:"encrypted_private_key"`
	Certificate         Cert          `json:"certificate"`
	ClearPassphrase     []byte        `json:"-"`
	EncryptedPassphrase []byte        `json:"encrypted_passphrase"`
	Salt                []byte        `json"salt"`
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

func NewUser(identifiers ...interface{}) (*User, error) {
	idents := []*Identifier{}
	for i, v := range identifiers {
		ident := Identifier{}
		if i%2 == 0 {
			ident.Name = expectString(v)
			if ident.Name == "" {
				return nil, ErrNewUser(i)
			}
			continue
		}
		num := expectUint(v)
		if num == -1 {
			return nil, ErrNewUser(i)
		}
		ident.Type = uint(num)
		idents = append(idents, &ident)
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
			err = ErrBadPassword{}
		}
	}
	return err
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

func (u *User) decrypt_privatekey() {
	key := append(u.Salt, u.ClearPassphrase...)
	data, err := x509.DecryptPEMBlock(u.EncryptedPrivateKey, key)
	if err != nil {
		return
	}
	k, err := x509.ParseECPrivateKey(data)
	if err != nil {
		return
	}
	u.ClearPrivateKey = (*PrivateKey)(k)
}

func (u *User) encrypt_privatekey() {
	key := append(u.Salt, u.ClearPassphrase...)
	kb, err := x509.MarshalECPrivateKey((*ecdsa.PrivateKey)(u.ClearPrivateKey))
	if err != nil {
		return
	}
	blk, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", kb, key, x509.PEMCipherAES256)
	if err != nil {
		return
	}
	u.EncryptedPrivateKey = blk
}

func (u *User) manage_keys() {
	if u.ClearPrivateKey == nil && u.EncryptedPrivateKey == nil {
		return
	}
	if u.ClearPrivateKey != nil && u.EncryptedPrivateKey != nil {
		return
	}

	if u.ClearPrivateKey != nil {
		u.encrypt_privatekey()
		return
	}

	u.decrypt_privatekey()
}

func (e *User) ID() *c4.ID {
	return nil
}

func (e *User) Name() string {
	var out []string
	for _, v := range e.Identities {
		out = append(out, v.Name)
	}
	return strings.Join(out, ",")
}

func (e *User) GenerateKeys() error {
	pri, _, err := generateKeys()
	if err != nil {
		return err
	}
	e.ClearPrivateKey = (*PrivateKey)(pri)
	return nil
}

func (e *User) Public() *PublicKey {
	if e.ClearPrivateKey != nil {
		return e.ClearPrivateKey.Public()
	}
	return nil
}

func (e *User) Private() *PrivateKey {
	return e.ClearPrivateKey
}

func (e *User) Sign(id *c4.ID) (*Signature, error) {
	return NewSignature(e.ClearPrivateKey, id)
}

func (e *User) SetCert(cert Cert) {
	e.Certificate = cert
}

func (e *User) Cert() Cert {
	return e.Certificate
}

func (e *User) TLScert(t TLScertType) (tls.Certificate, error) {
	return tls.X509KeyPair(e.Certificate.PEM(), e.Private().PEM())
}

func (e *User) Endorse(target Entity) (Cert, error) {
	return endorse(e, target)
}

func expectString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	}
	return ""
}

func expectUint(v interface{}) int {
	switch num := v.(type) {
	case uint:
		return int(num)
	}
	return -1
}
