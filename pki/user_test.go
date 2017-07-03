package pki_test

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cheekybits/is"

	"github.com/Avalanche-io/c4/pki"
)

func TestUserSaveLoad(t *testing.T) {
	is := is.New(t)
	u1, err := pki.NewUser("john.doe@example.com", pki.EMail)
	is.NoErr(err)
	is.NotNil(u1)
	is.NoErr(u1.GenerateKeys())

	is.NoErr(u1.Passphrase("some passphrase"))

	// Save
	data, err := json.Marshal(u1)
	is.NoErr(err)

	// Load
	var u2 pki.User
	is.NoErr(json.Unmarshal(data, &u2))

	// Does not save the PrivateKey, or Passphrase in the clear
	is.NotNil(u2.EncryptedPrivateKey)
	is.Nil(u2.ClearPrivateKey)
	is.NotNil(u2.EncryptedPassphrase)
	is.Nil(u2.ClearPassphrase)

	is.NoErr(u2.Passphrase("some passphrase"))
	is.NotNil(u2.Private())

	var u3 pki.User
	is.NoErr(json.Unmarshal(data, &u3))

	err = u3.Passphrase("wrong passphrase")
	is.Err(err)
	is.Equal(err.Error(), "incorrect passphrase")

	is.Nil(u3.Private())
}

func TestUserCSR(t *testing.T) {
	is := is.New(t)

	// Create a Certificate Authority
	ca, err := pki.CreateAthorty(pkix.Name{CommonName: "c4.studio.com"}, nil, nil)
	is.NoErr(err)
	is.NotNil(ca)

	// Create Domain Entity
	server, err := pki.NewDomain("test")
	server.AddIPs(net.ParseIP("127.0.0.1"))
	is.NoErr(err)
	err = server.GenerateKeys()
	is.NoErr(err)

	servercsr, err := server.CSR()
	is.NoErr(err)
	is.NotNil(servercsr)

	// CA endorses the domain's CSR.
	serverCert, err := ca.Approve(servercsr)
	is.NoErr(err)
	is.NotNil(serverCert)

	server.SetCert(serverCert)

	// Create a user
	user, err := pki.NewUser("john.doe@example.com", pki.EMail)
	is.NoErr(err)
	is.NotNil(user)
	is.NoErr(user.GenerateKeys())
	user.Passphrase("some passphrase")

	csr, err := user.CSR()
	is.NoErr(err)
	is.NotNil(csr)

	// CA endorses the users Certificate Signing Request.
	userCert, err := ca.Approve(csr)
	is.NoErr(err)
	is.NotNil(userCert)

	user.SetCert(userCert)

	// Test if user cert can be used for TLS connection.
	message := []byte("Hello, C4!")
	hello := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(message)
	})
	// Create a pool of trusted certs which include the root CA
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(ca.Cert().PEM())

	servTLSCert, err := server.TLScert(pki.TLS_CLISRV)
	is.NoErr(err)

	// Require client authentication
	webserver := httptest.NewUnstartedServer(hello)
	webserver.TLS = &tls.Config{
		Certificates: []tls.Certificate{servTLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	}

	// Produce TLS credentials for client.
	clientTLSCert, err := user.TLScert(pki.TLS_CLIONLY)
	is.NoErr(err)

	// Create a client with
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      certPool,
				Certificates: []tls.Certificate{clientTLSCert},
			},
		},
	}

	// Start the server
	webserver.StartTLS()

	// Have client make Get request
	resp, err := client.Get(webserver.URL)
	is.NoErr(err)

	// Close server
	webserver.Close()

	// Read and check response
	reply := make([]byte, resp.ContentLength)
	body := resp.Body
	_, err = body.Read(reply)
	if err != nil {
		is.Equal(err, io.EOF)
	}
	is.Equal(reply, message)

	data, err := json.Marshal(user)
	is.NoErr(err)
	var user2 pki.User
	err = json.Unmarshal(data, &user2)
	is.NoErr(err)
}

func TestUserChangePassphrase(t *testing.T) {
	is := is.New(t)
	user, err := pki.NewUser("john.doe@example.com", pki.EMail)
	is.NoErr(err)
	is.NotNil(user)
	is.NoErr(user.GenerateKeys())
	is.NotNil(user.Private())
	oldpw := "some passphrase"
	// set original passphrase
	err = user.Passphrase(oldpw)
	is.NoErr(err)

	newpw := "new passphrase"
	err = user.ChangePassphrase(oldpw, newpw)
	is.NoErr(err)

	// Save
	data, err := json.Marshal(user)
	is.NoErr(err)

	// Load
	var user2 pki.User
	is.NoErr(json.Unmarshal(data, &user2))

	is.Nil(user2.Private())
	is.NoErr(user2.Passphrase(newpw))
	is.NotNil(user2.Private())
}

func TestUserLogout(t *testing.T) {
	is := is.New(t)
	user, err := pki.NewUser("john.doe@example.com", pki.EMail)
	is.NoErr(err)
	is.NotNil(user)
	is.NoErr(user.GenerateKeys())
	is.NotNil(user.Private())
	oldpw := "some passphrase"

	err = user.Passphrase(oldpw)
	is.NoErr(err)

	is.NotNil(user.EncryptedPrivateKey)
	is.NotNil(user.ClearPrivateKey)
	is.NotNil(user.EncryptedPassphrase)
	is.NotNil(user.ClearPassphrase)

	user.Logout()
	is.NoErr(err)
	is.NotNil(user.EncryptedPrivateKey)
	is.Nil(user.ClearPrivateKey)
	is.NotNil(user.EncryptedPassphrase)
	is.Nil(user.ClearPassphrase)

	err = user.Passphrase(oldpw)
	is.NoErr(err)
	is.NotNil(user.EncryptedPrivateKey)
	is.NotNil(user.ClearPrivateKey)
	is.NotNil(user.EncryptedPassphrase)
	is.NotNil(user.ClearPassphrase)
}
