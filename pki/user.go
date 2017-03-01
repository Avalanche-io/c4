package pki

import (
	"crypto/tls"
	"fmt"
	"strings"

	c4 "github.com/Avalanche-io/c4/id"
)

// User is an Entity that represents a a human user.
type User struct {
	Identities []*Identifier
	pri        *PrivateKey
	pub        *PublicKey
	cert       Cert //cert       *standardCert
}

type NewUserError int

func (e NewUserError) Error() string {
	return fmt.Sprintf("expected non empty string in argument %d", e)
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
				return nil, NewUserError(i)
			}
			continue
		}
		num := expectUint(v)
		if num == -1 {
			return nil, NewUserError(i)
		}
		ident.Type = uint(num)
		idents = append(idents, &ident)
	}
	u := User{
		Identities: idents,
	}
	return &u, nil
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
	pri, pub, err := generateKeys()
	if err != nil {
		return err
	}
	e.pri = (*PrivateKey)(pri)
	e.pub = (*PublicKey)(pub)
	return nil
}

func (e *User) Public() *PublicKey {
	return e.pub
}

func (e *User) SetPublic(key *PublicKey) {
	e.pub = key
}

func (e *User) Private() *PrivateKey {
	return e.pri
}

func (e *User) Sign(id *c4.ID) (*Signature, error) {
	return NewSignature(e.pri, id)
}
func (e *User) Verify(sig *Signature) bool {
	return sig.Varify(e)
}

func (e *User) TLScert(t TLScertType) (tls.Certificate, error) {
	return tls.X509KeyPair(e.cert.PEM(), e.Private().PEM())
}

func (e *User) Endorse(target Entity) (Cert, error) {
	return endorse(e, target)
}

func (e *User) SetCert(c Cert) {
	e.cert = c
}

func (e *User) Cert() Cert {
	return e.cert
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
