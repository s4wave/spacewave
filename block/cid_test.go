package block

import (
	"encoding/hex"
	"github.com/aperturerobotics/bifrost/hash"
	"testing"
)

// TestMashalKeyConsistent ensures the hash type marshaling is consistent
func TestMarshalKeyConsistent(t *testing.T) {
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	c := NewBlockRef(h)
	mk, err := c.MarshalKey()
	if err != nil {
		t.Fatal(err.Error())
	}

	if hex.EncodeToString(mk) != "0a24080112209f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Fail()
	}
}
