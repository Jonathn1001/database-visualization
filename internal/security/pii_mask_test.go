package security

import "testing"

func TestMaskValueColumnAllowlist(t *testing.T) {
	cases := []struct {
		col string
		val any
	}{
		{"email", "john@example.com"},
		{"EMAIL", "john@example.com"},
		{"ssn", "123-45-6789"},
		{"credit_card", "4111111111111111"},
		{"iban", "DE89370400440532013000"},
	}
	for _, c := range cases {
		if got := MaskValue(c.col, c.val); got != "***" {
			t.Errorf("MaskValue(%q,%v) = %v, want ***", c.col, c.val, got)
		}
	}
	if got := MaskValue("email", nil); got != nil {
		t.Errorf("nil value should stay nil, got %v", got)
	}
}

func TestMaskValueEmailRegex(t *testing.T) {
	// A non-allowlisted column holding an email gets partial masking.
	got := MaskValue("notes", "john@example.com")
	if got != "j***@***.com" {
		t.Errorf("email mask = %v, want j***@***.com", got)
	}
}

func TestMaskValuePhoneRegex(t *testing.T) {
	got, ok := MaskValue("contact", "+1 (415) 555-1234").(string)
	if !ok {
		t.Fatalf("expected string, got %T", got)
	}
	if got[len(got)-4:] != "1234" {
		t.Errorf("last 4 digits should be visible: %q", got)
	}
	if countDigits(got) != 4 {
		t.Errorf("only last 4 digits should remain, got %d in %q", countDigits(got), got)
	}
}

func TestMaskValuePassthrough(t *testing.T) {
	if got := MaskValue("name", "Alice"); got != "Alice" {
		t.Errorf("non-PII passthrough failed: %v", got)
	}
	if got := MaskValue("age", 42); got != 42 {
		t.Errorf("non-string passthrough failed: %v", got)
	}
}

func TestMaskRowsCopies(t *testing.T) {
	rows := []map[string]any{{"email": "a@b.com", "name": "Bob"}}
	out := MaskRows(rows)
	if out[0]["email"] != "***" {
		t.Errorf("email not masked: %v", out[0]["email"])
	}
	if out[0]["name"] != "Bob" {
		t.Errorf("name should be untouched: %v", out[0]["name"])
	}
	// Original must be untouched.
	if rows[0]["email"] != "a@b.com" {
		t.Errorf("MaskRows mutated input: %v", rows[0]["email"])
	}
}
