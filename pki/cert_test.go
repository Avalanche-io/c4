package pki_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cheekybits/is"

	c4k "github.com/Avalanche-io/c4/pki"
)

func TestCreateC4dCert(t *testing.T) {
	is := is.New(t)
	_ = is
	message := []byte("Hello, C4!")
	hello := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(message)
	})
	// Create a Certificate Authority
	rootEntity, err := c4k.CreateCA("c4.studio.com")
	is.NoErr(err)
	is.NotNil(rootEntity)

	// Create a pool of trusted certs which include the root CA
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(rootEntity.Cert().PEM()) //rootCertPEM)

	// Create Domain Entity
	serverEntity, err := c4k.NewDomain("c4.example.com")
	is.NoErr(err)
	// Generate private public key pairs for Domain
	err = serverEntity.GenerateKeys()
	is.NoErr(err)

	// Create Client Entity
	clientEntity, err := c4k.NewDomain("localhost")
	is.NoErr(err)
	err = clientEntity.GenerateKeys()
	is.NoErr(err)

	// Have root endorse the server.
	serverCert, err := rootEntity.Endorse(serverEntity)
	is.NoErr(err)
	is.NotNil(serverCert)

	// Have root endorse the client.
	clientCert, err := rootEntity.Endorse(clientEntity)
	is.NoErr(err)
	is.NotNil(clientCert)

	// Produce TLS credentials for server.
	servTLSCert, err := serverEntity.TLScert(c4k.TLS_CLISRV)
	is.NoErr(err)

	// Produce TLS credentials for client.
	clientTLSCert, err := clientEntity.TLScert(c4k.TLS_CLIONLY)
	is.NoErr(err)

	// Create a server with client validation using the server TLS credentials.
	s := httptest.NewUnstartedServer(hello)
	s.TLS = &tls.Config{
		Certificates: []tls.Certificate{servTLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	}

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
	s.StartTLS()

	// Have client make Get request
	resp, err := client.Get(s.URL)
	is.NoErr(err)

	// Close server
	s.Close()

	// Read and check response
	reply := make([]byte, resp.ContentLength)
	body := resp.Body
	_, err = body.Read(reply)
	if err != nil {
		is.Equal(err, io.EOF)
	}
	is.Equal(reply, message)
}
