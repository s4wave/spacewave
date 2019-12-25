package padding

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestPadUnpad(t *testing.T) {
	data := make([]byte, 27)
	rand.Read(data)
	og := make([]byte, len(data))
	copy(og, data)
	padded := PadInPlace(data)
	if len(padded) != 32 {
		t.Fail()
	}
	unpadded, err := UnpadInPlace(padded)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(unpadded, og) {
		t.Fatalf("pad unpad fail: %v != %v", unpadded, padded)
	}
}
