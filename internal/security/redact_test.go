package security

import "testing"

func TestRedactValue(t *testing.T) {
	got := RedactValue("bearer abcdefghijklmnop")
	if got == "bearer abcdefghijklmnop" || got == "" {
		t.Fatalf("expected redacted value, got %q", got)
	}
}

func TestRedactKeyValue(t *testing.T) {
	got := RedactKeyValue("client_secret", "super-secret")
	if got == "super-secret" {
		t.Fatal("expected sensitive key to be redacted")
	}
	if RedactKeyValue("page", 1) != 1 {
		t.Fatal("expected non-sensitive key to pass through")
	}
}
