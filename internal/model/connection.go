package model

import (
	"net/url"
	"regexp"
)

// ConnectionConfig is the user-supplied configuration for opening a database
// connection. The password is never logged (see obs redaction) and is not
// persisted in the connection pool (§22.7).
type ConnectionConfig struct {
	Engine   string `json:"engine"` // "postgres" | "mongodb" | "sqlite"
	DSN      string `json:"dsn,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	// FilePath is used by file-based engines (SQLite).
	FilePath string `json:"filePath,omitempty"`
	// Label is an optional human-friendly name for the connection.
	Label string `json:"label,omitempty"`
}

// Redacted returns a copy of the config safe for logging and API responses.
// It removes the Password field AND any password embedded in the DSN — a DSN
// like "postgres://user:secret@host/db" would otherwise leak the credential
// through API responses and the in-memory pool.
func (c ConnectionConfig) Redacted() ConnectionConfig {
	c.Password = ""
	c.DSN = RedactDSN(c.DSN)
	return c
}

// keywordPwRe matches the password in libpq keyword-style DSNs
// ("host=... password=secret ...").
var keywordPwRe = regexp.MustCompile(`(?i)\b(password|pwd)=(?:'[^']*'|\S+)`)

// RedactDSN replaces any password in a DSN with "***", handling both URL form
// (scheme://user:pass@host) and libpq keyword form (password=secret).
func RedactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		if _, hasPw := u.User.Password(); hasPw {
			u.User = url.UserPassword(u.User.Username(), "REDACTED")
			return u.String()
		}
	}
	return keywordPwRe.ReplaceAllString(dsn, "${1}=REDACTED")
}
