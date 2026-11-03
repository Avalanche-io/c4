//go:build go1.21
// +build go1.21

package store

import (
	"strings"
	"testing"
	"time"
)

// Logged through slog, Credentials shows the key and never the secrets.
func TestCredentialsLogValueHidesSecrets(t *testing.T) {
	c := Credentials{AccessKeyID: "AK", SecretAccessKey: "SECRETVAL", SessionToken: "TOKVAL", Expires: time.Unix(1800000000, 0)}
	s := c.LogValue().String()
	if strings.Contains(s, "SECRETVAL") || strings.Contains(s, "TOKVAL") || !strings.Contains(s, "AK") {
		t.Errorf("logged as %s", s)
	}
}
