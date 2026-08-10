package main

import "testing"

func TestVerifyActualGeneratedCiphertext(t *testing.T) {
	key := "kfQr45YcMPmurYjvdZaeLR3O1Ne5YUZoJEX4GXc9zsc="

	from, err := decryptAESGCM("3SPk/l7jQoZKcOO5+QCI9jGj7WxGq6w2YihTGlfpbS63iFU6YtV9sg==", key)
	if err != nil {
		t.Fatalf("decrypt from_acct_no error: %v", err)
	}
	if from != "417790530860" {
		t.Fatalf("from_acct_no: got %q, want 417790530860", from)
	}

	to, err := decryptAESGCM("/QVFzhHryf48sfZ6k7VwLAef3qssRmJHv3eTYy2ePL1eCSdpDqE=", key)
	if err != nil {
		t.Fatalf("decrypt to_acct_no error: %v", err)
	}
	if to != "6707736071" {
		t.Fatalf("to_acct_no: got %q, want 6707736071", to)
	}
	t.Logf("from_acct_no=%s to_acct_no=%s", from, to)
}
