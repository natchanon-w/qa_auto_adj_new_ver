package main

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := "kfQr45YcMPmurYjvdZaeLR3O1Ne5YUZoJEX4GXc9zsc="
	plain := "170000002110001"

	ct, err := encryptAESGCM(plain, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if ct == plain {
		t.Fatalf("ciphertext equals plaintext, encryption did nothing")
	}

	pt, err := decryptAESGCM(ct, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if pt != plain {
		t.Fatalf("round-trip mismatch: got %q, want %q", pt, plain)
	}
}

func TestEncryptEmptyStringNoOp(t *testing.T) {
	ct, err := encryptAESGCM("", "kfQr45YcMPmurYjvdZaeLR3O1Ne5YUZoJEX4GXc9zsc=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "" {
		t.Fatalf("expected empty ciphertext for empty plaintext, got %q", ct)
	}
}
