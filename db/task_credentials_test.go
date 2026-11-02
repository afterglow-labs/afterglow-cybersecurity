package db

import (
	"strings"
	"testing"
)

func TestTaskCredentialEncryption(t *testing.T) {
	d := &DB{}
	if err := d.SetTaskCredentialKey([]byte(strings.Repeat("a", 32))); err != nil {
		t.Fatal(err)
	}
	sealed, err := d.sealCredential("unique test secret with spaces ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "unique test secret") || !strings.HasPrefix(sealed, "v1:") {
		t.Fatalf("secret was not encrypted: %q", sealed)
	}
	got, err := d.openCredential(sealed)
	if err != nil || got != "unique test secret with spaces " {
		t.Fatalf("round trip failed: %q, %v", got, err)
	}
	other := &DB{}
	_ = other.SetTaskCredentialKey([]byte(strings.Repeat("b", 32)))
	if _, err := other.openCredential(sealed); err == nil {
		t.Fatal("wrong installation key decrypted a credential")
	}
	if _, err := d.openCredential(sealed[:len(sealed)-2] + "xx"); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
}

func TestTaskCredentialValidation(t *testing.T) {
	valid, err := (TaskCredentialInput{Domain: " EXAMPLE.COM ", Username: "tester", Password: " leading space ", Source: "agent"}).normalize()
	if err != nil || valid.Domain != "example.com" || valid.Password != " leading space " {
		t.Fatalf("normalization changed account meaning: %+v, %v", valid, err)
	}
	if _, err := (TaskCredentialInput{Domain: "example.com/path", Username: "tester"}).normalize(); err == nil {
		t.Fatal("path-bearing domain accepted")
	}
	if _, err := (TaskCredentialInput{Domain: "example.com", Username: "tester\nadmin"}).normalize(); err == nil {
		t.Fatal("control character in username accepted")
	}
}
