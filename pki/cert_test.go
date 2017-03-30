package pki_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cheekybits/is"

	c4 "github.com/avalanche-io/c4/id"
	"github.com/avalanche-io/c4/pki"
)

// New and improved with sub-tests.
// TODO: Update other tests in the same stile.
func TestCreateC4dCert(t *testing.T) {
	var err error
	var rootEntity pki.Entity
	var clientEntity, serverEntity *pki.Domain
	message := []byte("Hello, C4!")

	t.Run("Create Certificate Authority", func(t *testing.T) {
		tis := is.New(t)
		rootEntity, err = pki.CreateCA("c4.studio.com")
		tis.NoErr(err)
		tis.NotNil(rootEntity)
	})

	t.Run("Create Domain Entity", func(t *testing.T) {
		tis := is.New(t)
		serverEntity, err = pki.NewDomain()
		serverEntity.AddDomains("c4.example.com")
		tis.NoErr(err)
		// Generate private public key pairs for Domain
		err = serverEntity.GenerateKeys()
		tis.NoErr(err)
	})

	t.Run("Create Client Entity", func(t *testing.T) {
		tis := is.New(t)
		// Create Client Entity
		clientEntity, err = pki.NewDomain()
		clientEntity.AddDomains("localhost")
		tis.NoErr(err)
		err = clientEntity.GenerateKeys()
		tis.NoErr(err)
	})

	var serverCert, clientCert *pki.Cert

	t.Run("Endorse Certificate Chain", func(t *testing.T) {
		tis := is.New(t)
		// Have root endorse the server.
		serverCert, err = rootEntity.Endorse(serverEntity)
		tis.NoErr(err)
		tis.NotNil(serverCert)

		// Have root endorse the client.
		clientCert, err = rootEntity.Endorse(clientEntity)
		tis.NoErr(err)
		tis.NotNil(clientCert)
	})

	t.Run("Http Client Server Authentication", func(t *testing.T) {
		tis := is.New(t)

		// Create a pool of trusted certs which include the root CA
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(rootEntity.Cert().PEM())

		// Produce TLS credentials for server.
		servTLSCert, err := serverEntity.TLScert(pki.TLS_CLISRV)
		tis.NoErr(err)

		// Produce TLS credentials for client.
		clientTLSCert, err := clientEntity.TLScert(pki.TLS_CLIONLY)
		tis.NoErr(err)

		hello := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(message)
		})

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
		tis.NoErr(err)

		// Close server
		s.Close()
		// Read and check response
		reply := make([]byte, resp.ContentLength)
		body := resp.Body
		_, err = body.Read(reply)
		if err != nil {
			tis.Equal(err, io.EOF)
		}
		tis.Equal(reply, message)
	})
}

func TestCertSigningRequest(t *testing.T) {
	tis := is.New(t)

	// Create Domain Entity
	serverEntity, err := pki.NewDomain()
	serverEntity.AddDomains("c4.example.com")
	tis.NoErr(err)
	// Generate private public key pairs for Domain
	err = serverEntity.GenerateKeys()
	tis.NoErr(err)

	// csr, err := serverEntity.CSR("foo", "Foo corp.", "U.S.")
	csr, err := serverEntity.CSR()
	tis.NoErr(err)
	tis.NotNil(csr)

	// verify signature
	tis.True(csr.Varify(serverEntity))

	// manually verify signature
	cr := csr.CR()
	id := c4.Identify(bytes.NewReader(cr.RawTBSCertificateRequest))
	ecdsaSig := new(struct{ R, S *big.Int })
	_, err = asn1.Unmarshal(cr.Signature, ecdsaSig)
	tis.NoErr(err)
	tis.True(ecdsa.Verify((*ecdsa.PublicKey)(serverEntity.Public()), id.Digest(), ecdsaSig.R, ecdsaSig.S))

	// test parsing
	csr2, err := pki.ParseCertificateRequest(csr.DER())
	tis.NoErr(err)
	tis.True(csr2.Varify(serverEntity))

	// manually verify parsed request signature
	cr2 := csr2.CR()
	id2 := c4.Identify(bytes.NewReader(cr.RawTBSCertificateRequest))
	ecdsaSig2 := new(struct{ R, S *big.Int })
	_, err = asn1.Unmarshal(cr2.Signature, ecdsaSig2)
	tis.NoErr(err)
	tis.True(ecdsa.Verify((*ecdsa.PublicKey)(serverEntity.Public()), id2.Digest(), ecdsaSig2.R, ecdsaSig2.S))
}

// func TestSave(t *testing.T) {
