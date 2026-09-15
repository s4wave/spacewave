package transform_gzip

import (
	"bytes"
	"testing"
)

// BenchmarkGzipCursorLifetime includes codec construction as remote World
// cursors do, and checks that separately constructed codecs share no input.
func BenchmarkGzipCursorLifetime(b *testing.B) {
	data := bytes.Repeat([]byte("object body "), 64)
	b.ReportAllocs()
	for b.Loop() {
		encoder, err := NewGzip(&Config{})
		if err != nil {
			b.Fatal(err)
		}
		encoded, err := encoder.EncodeBlock(data)
		if err != nil {
			b.Fatal(err)
		}
		decoder, err := NewGzip(&Config{})
		if err != nil {
			b.Fatal(err)
		}
		decoded, err := decoder.DecodeBlock(encoded)
		if err != nil || !bytes.Equal(decoded, data) {
			b.Fatalf("round trip: %v", err)
		}
	}
}
