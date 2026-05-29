package model

import (
	"strings"
	"testing"
)

func TestRedactDSN(t *testing.T) {
	cases := []struct {
		name, in string
		secret   string // must NOT appear in output
		want     string // optional exact match; empty = skip
	}{
		{"url with password", "postgres://user:secret@host:5432/db", "secret", "postgres://user:REDACTED@host:5432/db"},
		{"url no password", "postgres://user@host:5432/db", "", "postgres://user@host:5432/db"},
		{"url no userinfo", "postgres://host:5432/db", "", "postgres://host:5432/db"},
		{"keyword form", "host=localhost user=u password=secret dbname=db", "secret", ""},
		{"keyword quoted", "host=localhost password='se cret' dbname=db", "se cret", ""},
		{"empty", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RedactDSN(c.in)
			if c.secret != "" && strings.Contains(got, c.secret) {
				t.Errorf("RedactDSN(%q) = %q still contains secret %q", c.in, got, c.secret)
			}
			if c.want != "" && got != c.want {
				t.Errorf("RedactDSN(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRedactedStripsBothFields(t *testing.T) {
	c := ConnectionConfig{
		Password: "secret",
		DSN:      "postgres://user:secret@host/db",
	}
	r := c.Redacted()
	if r.Password != "" {
		t.Errorf("Password not cleared: %q", r.Password)
	}
	if strings.Contains(r.DSN, "secret") {
		t.Errorf("DSN still leaks password: %q", r.DSN)
	}
}
