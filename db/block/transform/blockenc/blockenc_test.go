package transform_blockenc

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/util/blockenc"
)

func TestBlockEncRoundTripUsesLazyPool(t *testing.T) {
	// Configure the encoder with a valid key and pooled block codec.
	key := bytes.Repeat([]byte{1}, 32)
	enc, err := NewBlockEnc(&Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      key,
	})
	if err != nil {
		t.Fatalf("NewBlockEnc: %v", err)
	}

	// Encode the payload and confirm ciphertext differs from plaintext.
	body := []byte("payload")
	encoded, err := enc.EncodeBlock(body)
	if err != nil {
		t.Fatalf("EncodeBlock: %v", err)
	}
	if bytes.Equal(encoded, body) {
		t.Fatal("encoded block should differ from plaintext")
	}

	// Decode the ciphertext and confirm the original payload is restored.
	decoded, err := enc.DecodeBlock(encoded)
	if err != nil {
		t.Fatalf("DecodeBlock: %v", err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatalf("decoded block = %q, want %q", decoded, body)
	}
}

// TestBlockEncAuthenticates checks which ciphers prove the decoded bytes.
func TestBlockEncAuthenticates(t *testing.T) {
	// The no-op ciphers leave bytes untouched. Each sealed cipher proves them.
	tests := []struct {
		cipher blockenc.BlockEnc
		want   bool
	}{
		{blockenc.BlockEnc_BlockEnc_NONE, false},
		{blockenc.BlockEnc_BlockEnc_UNKNOWN, false},
		{blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305, true},
		{blockenc.BlockEnc_BlockEnc_SECRET_BOX, true},
		{blockenc.BlockEnc_BlockEnc_AES_256_GCM, true},
	}
	for _, tt := range tests {
		t.Run(tt.cipher.String(), func(t *testing.T) {
			// Construct the same step a configured decoder would use.
			enc, err := NewBlockEnc(&Config{BlockEnc: tt.cipher, Key: make([]byte, blockenc.BlockEnc_KeySize[tt.cipher])})
			if err != nil {
				t.Fatal(err)
			}

			// Check the step's claim about a successful decode.
			if got := enc.Authenticates(); got != tt.want {
				t.Fatalf("Authenticates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewBlockEncRejectsInvalidConfigBeforePoolUse(t *testing.T) {
	_, err := NewBlockEnc(&Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_SECRET_BOX,
		Key:      []byte("short"),
	})
	if err == nil {
		t.Fatal("NewBlockEnc should reject invalid key size")
	}
}
