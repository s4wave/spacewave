package blockenc

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/pkg/errors"
)

func TestBlockEnc(t *testing.T) {
	// Generate a random key shared by the block-encryption methods.
	var pass [32]byte
	randomize := func(dat []byte) {
		if _, err := rand.Read(dat); err != nil {
			t.Fatal(err.Error())
		}
	}
	randomize(pass[:])

	// Exercise each block-encryption method with pooled buffers.
	alloc, relBuf := NewPoolAlloc()
	for i := BlockEnc_BlockEnc_XCHACHA20_POLY1305; i <= BlockEnc_BlockEnc_MAX; i++ {
		if err := func() error {
			// Construct the selected block-encryption method with the test key.
			c, err := BuildBlockEnc(i, pass[:])
			if err != nil {
				return err
			}

			// Prepare random plaintext and preserve its bytes for the round trip.
			data := alloc(128)
			randomize(data[:])
			dataBefore := make([]byte, len(data))
			copy(dataBefore, data[:])

			// Encrypt the plaintext and release its pooled source buffer.
			out, err := c.Encrypt(alloc, data[:])
			relBuf(data)
			if err != nil {
				return err
			}

			// Verify the encrypted block differs from the original plaintext.
			if bytes.Equal(out[:], dataBefore) {
				return errors.New("data was identical after encrypt")
			}

			// Decrypt the encrypted block using the same method and key.
			data, err = c.Decrypt(alloc, out)
			if err != nil {
				return err
			}

			// Verify the decrypted block restores the complete plaintext.
			if !bytes.Equal(data[:], dataBefore) {
				return errors.Errorf("data was not identical after decrypt: %v != expected %v", data, dataBefore)
			}

			return nil
		}(); err != nil {
			t.Fatalf("block enc %v: %v", i.String(), err)
		}
	}
}

func TestDefaultBlockEncIsAES256GCM(t *testing.T) {
	if DefaultBlockEnc != BlockEnc_BlockEnc_AES_256_GCM {
		t.Fatalf("DefaultBlockEnc = %s, want BlockEnc_AES_256_GCM", DefaultBlockEnc.String())
	}
	if err := ValidateKeySize(DefaultBlockEnc, 32); err != nil {
		t.Fatalf("ValidateKeySize(DefaultBlockEnc): %v", err)
	}
}

func TestDeriveNonceSHA256Fixture(t *testing.T) {
	var nonce [12]byte
	DeriveNonceSHA256([]byte("spacewave blockenc nonce fixture"), nonce[:])
	if got, want := hex.EncodeToString(nonce[:]), "76f4a5c5138c6e104b372532"; got != want {
		t.Fatalf("nonce = %s, want %s", got, want)
	}
}
