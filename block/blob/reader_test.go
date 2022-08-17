package blob

import (
	"bytes"
	"io"
	"testing"
)

// TestRawBlobReadSeek tests reading / seeking a raw blob.
func TestRawBlobReadSeek(t *testing.T) {
	b1 := buildMockRawBlob()
	testData := make([]byte, len(b1.RawData))
	copy(testData, b1.RawData)
	rdr := NewRawReader(b1)
	dat, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(testData, dat) {
		t.Fail()
	}
	n, err := rdr.Seek(0, io.SeekStart)
	if err != nil || n != 0 {
		if err == nil {
			t.Fail()
		} else {
			t.Fatal(err.Error())
		}
	}
	buf := make([]byte, 4)
	nx, err := rdr.Read(buf)
	if err != nil || nx != 4 {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(buf, testData[:len(buf)]) {
		t.Fail()
	}
}
