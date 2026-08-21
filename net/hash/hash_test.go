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

func TestRecommendedHashType(t *testing.T) {
	if RecommendedHashType != HashType_HashType_SHA256 {
		t.Fatalf("expected RecommendedHashType SHA256, got %s", RecommendedHashType.String())
	}
}

func TestUnsupportedHashTypeErrorClassifiesUnknownCodes(t *testing.T) {
	err := HashType(999).Validate()
	if err == nil {
		t.Fatal("expected unsupported hash type error")
	}
	if !errors.Is(err, ErrHashTypeUnsupported) {
		t.Fatalf("Validate() error = %v, want ErrHashTypeUnsupported", err)
	}
	var unsupported *UnsupportedHashTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Validate() error = %T, want UnsupportedHashTypeError", err)
	}
	if unsupported.HashType != HashType(999) {
		t.Fatalf("unsupported hash type = %v, want 999", unsupported.HashType)
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

// TestCompareHash tests hash equality across nil, type, length, and content.
func TestCompareHash(t *testing.T) {
	h1 := &Hash{HashType: HashType_HashType_BLAKE3, Hash: []byte{1, 2, 3}}
	h1Copy := &Hash{HashType: h1.GetHashType(), Hash: []byte{1, 2, 3}}
	h2 := &Hash{HashType: h1.GetHashType(), Hash: []byte{1, 2}}
	h3 := &Hash{HashType: HashType_HashType_SHA256, Hash: []byte{1, 2, 3}}

	cases := map[string]struct {
		a, b  *Hash
		equal bool
	}{
		"both nil":       {nil, nil, true},
		"left nil":       {nil, h1, false},
		"right nil":      {h1, nil, false},
		"equal":          {h1, h1Copy, true},
		"length differs": {h1, h2, false},
		"type differs":   {h1, h3, false},
	}
	for name, tc := range cases {
		if tc.a.CompareHash(tc.b) != tc.equal {
			t.Errorf("%s: CompareHash = %v, want %v", name, !tc.equal, tc.equal)
		}
	}
}
