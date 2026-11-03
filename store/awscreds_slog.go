//go:build go1.21

package store

import "log/slog"

// LogValue logs Credentials as String does: the key, never the secret
// or the token. In its own file so the package still builds on the
// Go 1.16 matrix, which has no log/slog.
func (c Credentials) LogValue() slog.Value { return slog.StringValue(c.String()) }
