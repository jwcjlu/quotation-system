package data

import (
	"crypto/rand"
	"testing"
)

func TestScriptAuthCipherRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	c, err := newScriptAuthCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := "p@ss w0rd 测试"
	enc, err := c.encryptString(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain {
		t.Fatal("expected ciphertext")
	}
	got, err := c.decryptString(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestScriptAuthCipherWrongKey(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	if _, err := rand.Read(k1); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(k2); err != nil {
		t.Fatal(err)
	}
	c1, err := newScriptAuthCipher(k1)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := newScriptAuthCipher(k2)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := c1.encryptString("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c2.decryptString(enc); err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}
