package block

import (
	"encoding/hex"
	"testing"

	"github.com/aperturerobotics/bifrost/hash"
)

// TestMashalKeyConsistent ensures the hash type marshaling is consistent
func TestMarshalKeyConsistent(t *testing.T) {
	h, err := hash.Sum(DefaultHashType, []byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	c := NewBlockRef(h)
	mk, err := c.MarshalKey()
	if err != nil {
		t.Fatal(err.Error())
	}

	expected := "0a24080312204878ca0425c739fa427f7eda20fe845f6b2e46ba5fe2a14df5b1e32f50603215"
	if v := hex.EncodeToString(mk); v != expected {
		t.Fatalf("unexpected value: %s", v)
	}
}
