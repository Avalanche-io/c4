package pki

import "fmt"

type ErrNoValidCn struct{}

func (e ErrNoValidCn) Error() string {
	return "no domain names or ips to use as common name"
}

type ErrNewUser int

func (e ErrNewUser) Error() string {
	return fmt.Sprintf("expected non empty string in argument %d", e)
}

type ErrPrivateKeyPassword string

func (e ErrPrivateKeyPassword) Error() string {
	return "private key password " + string(e)
}

type ErrBadPassword struct{}

func (e ErrBadPassword) Error() string {
	return "incorrect password"
}
