package hash

import (
	"testing"

	"github.com/pkg/errors"
)

// TestVerifyData tests verifying some data with each hash type.
func TestVerifyData(t *testing.T) {
	data := []byte("hello world")
	for _, ht := range SupportedHashTypes {
		h, err := Sum(ht, data)
		werr := func(e error) error {
			return errors.Wrapf(e, "hash_type[%v]", ht)
		}
		if err != nil {
			t.Fatal(werr(err))
		}
		if _, err := h.VerifyData(data); err != nil {
			t.Fatal(werr(err))
		}
		t.Logf("OK: %s", ht.String())
	}
}

// TestJSON tests marshal and unmarshal hash from json.
func TestJSON(t *testing.T) {
	h, err := Sum(HashType_HashType_SHA256, []byte("hello world"))
	if err != nil {
		t.Fatal(err.Error())
	}
	jdata, err := h.MarshalJSON()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log(string(jdata))
	outHash, err := UnmarshalHashJSON(jdata)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !outHash.EqualVT(h) {
		t.Fail()
	}
}
