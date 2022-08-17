package auth_triplesec

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/keybase/go-triplesec"
	b58 "github.com/mr-tron/base58/base58"
)

// TestBasicEncryptDecrypt tests triplesec directly
func TestBasicEncryptDecrypt(t *testing.T) {
	password := []byte("hello world")
	salt := sha256.Sum256([]byte("testing-salt"))
	c, err := triplesec.NewCipher(password, salt[:16], triplesec.LatestVersion)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer c.Scrub()

	srcData := []byte("hello world 1234")
	srcCpy := make([]byte, len(srcData))
	copy(srcCpy, srcData)

	t.Logf("src len: %d", len(srcCpy))
	dst, err := c.Encrypt(srcData)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Logf("encrypted len: %d", len(dst))
	out, err := c.Decrypt(dst)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Logf("decrypted len: %d", len(out))
	if !bytes.Equal(out, srcCpy) {
		t.Fatal("out does not match src")
	}
}

// expectedE2EKey is the expected result of the end to end keygen.
var expectedE2EKey = `
7ga8UGbuqQP5mpiejSxZg4nccRWB5GjXiTbwQDSwPt7apAY943Pv3yXwbfmCvRsQG5h3JozQiTSZE
hcSjYZJuXLtKdpjTyHRAfTerxHczQfturJvGXVPcaH9ooNysaE3JCdxsrtVu72MbZ2dSPnn32vPhe
CkJ598NqBLiM38jqor56Sw7vJG4x7ngfZiyLB1Lu9rMVGGcLxVM3WqcLPP3oyHqKw
`

// TestEndToEnd tests an end to end usage.
func TestEndToEnd(t *testing.T) {
	salt, err := DeriveSalt([]byte("my-username"))
	if err != nil {
		t.Fatal(err.Error())
	}
	version := uint32(4)
	cipher, err := BuildCipher(version, salt, []byte("my-passphrase"))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cipher.Scrub()
	if err := VerifyCipher(cipher, salt); err != nil {
		t.Fatal(err.Error())
	}
	keyBytes, _, err := cipher.DeriveKey(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	keyb58 := b58.Encode(keyBytes)
	keyExpected := strings.ReplaceAll(expectedE2EKey, "\n", "")
	// ensure we deterministic generate key
	if keyb58 != keyExpected {
		t.Fatalf("expected key %s but got %s", keyExpected, keyb58)
	}
	t.Log(keyb58)
}
