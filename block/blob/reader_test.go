package blob

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"
)

// TestRawBlobReadSeek tests reading / seeking a raw blob.
func TestRawBlobReadSeek(t *testing.T) {
	b1 := buildMockRawBlob()
	testData := make([]byte, len(b1.RawData))
	copy(testData, b1.RawData)
	rdr := NewReader(context.Background(), nil, b1)
	dat, err := ioutil.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(testData, dat) != 0 {
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
	if bytes.Compare(buf, testData[:len(buf)]) != 0 {
		t.Fail()
	}
}
