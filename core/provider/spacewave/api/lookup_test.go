package provider_spacewave_api

import (
	"encoding/hex"
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
)

// lookupSObjectResponseHex is one LookupSObjectResponse as Cloud's lookup
// route sends it. Cloud's API tests hold the same bytes, so a field number or
// type that drifts between the two schemas fails in both repositories. The
// fixture uses only fields both sides know and leaves out the map, whose entry
// order the encoding does not fix.
const lookupSObjectResponseHex = "0a570a400a220a1a30316a30303030303030303030303030303030303030303030301a0461636374121a30316a303030303030303030303030303030303030303030303012090a05737061636518011a067368617265642801"

// TestLookupSObjectResponseEncoding checks the lookup reply's wire bytes in
// both directions.
func TestLookupSObjectResponseEncoding(t *testing.T) {
	const id = "01j00000000000000000000000"
	msg := &LookupSObjectResponse{Entry: &sobject.SharedObjectListEntry{
		Ref: &sobject.SharedObjectRef{
			ProviderResourceRef: &provider.ProviderResourceRef{Id: id, ProviderAccountId: "acct"},
			BlockStoreId:        id,
		},
		Meta:      &sobject.SharedObjectMeta{BodyType: "space", AccountPrivate: true},
		Source:    "shared",
		UpdatedAt: 1,
	}}
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(data); got != lookupSObjectResponseHex {
		t.Fatalf("encoding drifted:\n got %s\nwant %s", got, lookupSObjectResponseHex)
	}
	want, err := hex.DecodeString(lookupSObjectResponseHex)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &LookupSObjectResponse{}
	if err := decoded.UnmarshalVT(want); err != nil {
		t.Fatal(err)
	}
	if !decoded.EqualVT(msg) {
		t.Fatalf("decoded %v, want %v", decoded, msg)
	}
}
