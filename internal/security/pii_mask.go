// Package security implements read-only-tool safeguards: PII masking on sample
// data (§22.3) and (elsewhere) credential handling.
package security

import (
	"regexp"
	"strings"
)

// sensitiveColumns are masked entirely regardless of value (§22.3). Matching is
// case-insensitive on the exact column name.
var sensitiveColumns = map[string]bool{
	"email":           true,
	"phone":           true,
	"mobile":          true,
	"ssn":             true,
	"tax_id":          true,
	"passport":        true,
	"passport_number": true,
	"national_id":     true,
	"credit_card":     true,
	"cc_number":       true,
	"iban":            true,
	"swift":           true,
	"bank_account":    true,
}

const fullMask = "***"

var (
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	// A loose phone matcher: 7+ digits, optionally with +, spaces, dashes, parens.
	phoneRe = regexp.MustCompile(`^\+?[\d\s().-]{7,}$`)
	digitRe = regexp.MustCompile(`\d`)
)

// MaskRows returns a masked copy of the rows, leaving the input untouched.
func MaskRows(rows []map[string]any) []map[string]any {
	if rows == nil {
		return nil
	}
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		masked := make(map[string]any, len(row))
		for col, val := range row {
			masked[col] = MaskValue(col, val)
		}
		out[i] = masked
	}
	return out
}

// MaskValue masks a single column/value pair. A column-name match wins (full
// mask); otherwise the value is inspected for email/phone patterns and masked
// partially.
func MaskValue(column string, val any) any {
	if sensitiveColumns[strings.ToLower(column)] {
		if val == nil {
			return nil
		}
		return fullMask
	}
	s, ok := val.(string)
	if !ok {
		return val
	}
	switch {
	case emailRe.MatchString(s):
		return maskEmail(s)
	case phoneRe.MatchString(s) && countDigits(s) >= 7:
		return maskPhone(s)
	default:
		return val
	}
}

// maskEmail turns "john@example.com" into "j***@***.com".
func maskEmail(s string) string {
	at := strings.IndexByte(s, '@')
	local, domain := s[:at], s[at+1:]
	dot := strings.LastIndexByte(domain, '.')
	tld := domain[dot:] // includes leading dot

	head := "*"
	if len(local) > 0 {
		head = local[:1]
	}
	return head + "***@***" + tld
}

// maskPhone keeps the last 4 digits visible and masks the rest, preserving
// non-digit separators.
func maskPhone(s string) string {
	total := countDigits(s)
	seen := 0
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			seen++
			if seen > total-4 {
				b.WriteRune(r)
			} else {
				b.WriteByte('*')
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func countDigits(s string) int {
	return len(digitRe.FindAllString(s, -1))
}
