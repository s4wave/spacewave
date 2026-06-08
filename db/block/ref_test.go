package block

import (
	"strconv"
	"testing"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/net/hash"
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

	expected := "2W1M3RQW66FbepAmaPcufU8oiKnPhwL6AxpDdk6nqcKSAAitdX8B"
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
	// t.Log(string(jdata))
	outRef, err := UnmarshalBlockRefJSON(jdata)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !outRef.EqualVT(br) {
		t.Fail()
	}
}

func TestHashDefaults(t *testing.T) {
	if DefaultHashType != hash.HashType_HashType_SHA256 {
		t.Fatalf("expected default SHA256, got %s", DefaultHashType)
	}
}

func TestBuildBlockRefDefaultsToSHA256(t *testing.T) {
	ref, err := BuildBlockRef([]byte("test"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := ref.GetHash().GetHashType(); got != hash.HashType_HashType_SHA256 {
		t.Fatalf("expected SHA256, got %s", got)
	}
}

func TestPutOptsSelectHashType(t *testing.T) {
	if got := (*PutOpts)(nil).SelectHashType(0); got != hash.HashType_HashType_SHA256 {
		t.Fatalf("expected nil opts to select SHA256, got %s", got)
	}
	if got := (&PutOpts{}).SelectHashType(hash.HashType_HashType_SHA256); got != hash.HashType_HashType_SHA256 {
		t.Fatalf("expected store default SHA256 to win, got %s", got)
	}
	opts := &PutOpts{HashType: hash.HashType_HashType_SHA1}
	if got := opts.SelectHashType(hash.HashType_HashType_SHA256); got != hash.HashType_HashType_SHA1 {
		t.Fatalf("expected explicit opts SHA1 to win, got %s", got)
	}
	opts = &PutOpts{
		HashType: hash.HashType_HashType_SHA1,
		ForceBlockRef: &BlockRef{
			Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, nil),
		},
	}
	if got := opts.SelectHashType(hash.HashType_HashType_SHA256); got != hash.HashType_HashType_BLAKE3 {
		t.Fatalf("expected force ref BLAKE3 to win, got %s", got)
	}
}
