package service

import "testing"

func TestSecretBoxEncryptDecrypt(t *testing.T) {
	box, err := NewSecretBox("test-secret-with-enough-entropy")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Encrypt("sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "sk-test" {
		t.Fatal("expected encrypted secret to differ from plaintext")
	}
	decrypted, err := box.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "sk-test" {
		t.Fatalf("decrypted mismatch: got %q", decrypted)
	}
}

func TestSecretBoxRejectsWrongSecret(t *testing.T) {
	box, err := NewSecretBox("first-secret")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Encrypt("sk-test")
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewSecretBox("second-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(encrypted); err == nil {
		t.Fatal("expected wrong secret to fail")
	}
}
