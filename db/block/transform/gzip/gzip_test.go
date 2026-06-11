package transform_gzip

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

func TestDecodeBlockRejectsOversizedOutput(t *testing.T) {
	g, err := NewGzip(&Config{})
	if err != nil {
		t.Fatalf("NewGzip: %v", err)
	}

	encoded, err := g.EncodeBlock(bytes.Repeat([]byte("a"), block.MaxBlockSize+1))
	if err != nil {
		t.Fatalf("EncodeBlock: %v", err)
	}

	if _, err := g.DecodeBlock(encoded); err == nil {
		t.Fatal("expected oversized decoded block to fail")
	}
}
