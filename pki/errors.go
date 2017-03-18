package pki

import "fmt"

type ErrNoValidCn struct{}

func (e ErrNoValidCn) Error() string {
	return "no domain names or ips to use as common name"
}

type ErrNewUser int

func (e ErrNewUser) Error() string {
	return fmt.Sprintf("new user invalid argument %d", e)
}

type ErrPrivateKeyPassphrase string

func (e ErrPrivateKeyPassphrase) Error() string {
	return "private key passphrase " + string(e)
}

type ErrBadPassphrase struct{}

func (e ErrBadPassphrase) Error() string {
	return "incorrect passphrase"
}

type ErrBadCommonName struct{}

func (ErrBadCommonName) Error() string {
	return "invalid common name for certificate"
}

type ErrChangeNilPassphrase struct{}

func (ErrChangeNilPassphrase) Error() string {
	return "cannot change empty passphrase"
}
