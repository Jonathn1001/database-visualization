// Package obs configures structured logging for dbviz.
//
// All log output is routed through a slog handler that redacts credentials
// (see DESIGN_PLAN §17.3): any attribute whose key looks sensitive is replaced
// with "[REDACTED]", and any DSN-shaped value has its password stripped.
package obs

import (
	"io"
	"log/slog"
	"regexp"
	"strings"
)

// sensitiveKeys are attribute key substrings whose values must never be logged.
var sensitiveKeys = []string{"password", "pwd", "secret", "token", "dsn", "uri", "url"}

// dsnPasswordRe matches the user:password@ segment of a URI-style DSN.
var dsnPasswordRe = regexp.MustCompile(`://([^:/@]+):([^@/]+)@`)

// dsnShapeRe is a loose check for whether a value looks like a connection URI.
var dsnShapeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

// Setup builds a slog.Logger writing to output at the given level, with
// credential redaction applied to every attribute.
func Setup(level slog.Level, output io.Writer, text bool) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactSensitive,
	}
	var h slog.Handler
	if text {
		h = slog.NewTextHandler(output, opts)
	} else {
		h = slog.NewJSONHandler(output, opts)
	}
	return slog.New(h)
}

// redactSensitive is the slog ReplaceAttr hook. It redacts attributes with
// sensitive keys outright and scrubs passwords from DSN-shaped string values.
func redactSensitive(_ []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for _, s := range sensitiveKeys {
		if strings.Contains(key, s) {
			return slog.String(a.Key, "[REDACTED]")
		}
	}
	if a.Value.Kind() == slog.KindString {
		if v := a.Value.String(); looksLikeDSN(v) {
			return slog.String(a.Key, redactDSN(v))
		}
	}
	return a
}

// looksLikeDSN reports whether v has a URI scheme prefix.
func looksLikeDSN(v string) bool {
	return dsnShapeRe.MatchString(v)
}

// redactDSN replaces the password in a URI-style DSN with "***".
func redactDSN(dsn string) string {
	return dsnPasswordRe.ReplaceAllString(dsn, "://$1:***@")
}
