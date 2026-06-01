package transform_blockenc

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/util/blockenc"
)

func TestBlockEncRoundTripUsesLazyPool(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	enc, err := NewBlockEnc(&Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      key,
	})
	if err != nil {
		t.Fatalf("NewBlockEnc: %v", err)
	}

	body := []byte("payload")
	encoded, err := enc.EncodeBlock(body)
	if err != nil {
		t.Fatalf("EncodeBlock: %v", err)
	}
	if bytes.Equal(encoded, body) {
		t.Fatal("encoded block should differ from plaintext")
	}

	decoded, err := enc.DecodeBlock(encoded)
	if err != nil {
		t.Fatalf("DecodeBlock: %v", err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatalf("decoded block = %q, want %q", decoded, body)
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
