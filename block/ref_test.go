package block

import (
	"strconv"
	"testing"

	"github.com/aperturerobotics/bifrost/hash"
	b58 "github.com/mr-tron/base58/base58"
)

// TestBlockRef ensures the marshaling is consistent
func TestBlockRef(t *testing.T) {
	h, err := hash.Sum(DefaultHashType, []byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	c := NewBlockRef(h)
	mk, err := c.MarshalKey()
	if err != nil {
		t.Fatal(err.Error())
	}

	expected := "2W1M3cypWDWXjwjZKPFVoPEZtHwTBo7xzU1YH1mAoVd2b8jHjy3r"
	if v := b58.Encode(mk); v != expected {
		t.Fatalf("unexpected value: %s", v)
	}

	br, err := UnmarshalBlockRefJSON([]byte(strconv.Quote(expected)))
	if err != nil {
		t.Fatal(err.Error())
	}
	jdata, err := br.MarshalJSON()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log(string(jdata))
	outRef, err := UnmarshalBlockRefJSON(jdata)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !outRef.EqualVT(br) {
		t.Fail()
	}
}
