package main

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// c4dClient is the shared HTTP client for communicating with the local
// c4d daemon. It is lazily initialized from ~/.c4d/config.yaml on first
// use, including mTLS if the daemon is configured for it.
var c4dClient *http.Client

var c4dOnce sync.Once
var c4dCachedAddr string

func initC4dConnection() {
	c4dOnce.Do(func() {
		c4dClient, c4dCachedAddr = buildC4dClient()
	})
}

// c4dAddr returns the base URL for the local c4d daemon.
func c4dAddr() string {
	initC4dConnection()
	return c4dCachedAddr
}

// buildC4dClient reads ~/.c4d/config.yaml to determine the daemon address
// and TLS configuration. Falls back to C4D_ADDR env or http://localhost:17433.
func buildC4dClient() (*http.Client, string) {
	plainClient := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}

	fallbackAddr := "http://localhost:17433"
	if addr := os.Getenv("C4D_ADDR"); addr != "" {
		fallbackAddr = addr
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return plainClient, fallbackAddr
	}

	data, err := os.ReadFile(filepath.Join(home, ".c4d", "config.yaml"))
	if err != nil {
		return plainClient, fallbackAddr
	}

	var cfg c4dConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return plainClient, fallbackAddr
	}

	listen := cfg.Listen
	if listen == "" {
		return plainClient, fallbackAddr
	}

	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return plainClient, fallbackAddr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, port)

	certFile := expandHome(cfg.TLS.Cert, home)
	keyFile := expandHome(cfg.TLS.Key, home)
	caFile := expandHome(cfg.TLS.CA, home)

	if certFile == "" || keyFile == "" || caFile == "" {
		return plainClient, "http://" + addr
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return plainClient, "http://" + addr
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return plainClient, "http://" + addr
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)

	tlsClient := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 30 * time.Second,
			TLSClientConfig: &tls.Config{
				Certificates:       []tls.Certificate{cert},
				RootCAs:            caPool,
				InsecureSkipVerify: true, // c4d uses identity certs, not hostname certs
			},
		},
	}
	return tlsClient, "https://" + addr
}
