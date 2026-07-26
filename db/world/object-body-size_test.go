package world_test

import (
	"testing"

	world "github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// TestObjectBodyEncodedSizeMatchesGeneratedProto pins the manual page size
// accounting to the generated protobuf encoding, including the revision
// field, so budget checks cannot drift from the wire size.
func TestObjectBodyEncodedSizeMatchesGeneratedProto(t *testing.T) {
	cases := []*world.ObjectBody{
		{ObjectKey: "obj/a", Body: []byte("payload"), Rev: 7, Exists: true},
		{ObjectKey: "obj/b", Rev: 1 << 40, Exists: true},
		{ObjectKey: "obj/missing"},
	}
	for _, body := range cases {
		wire := &s4wave_world.ObjectBody{
			ObjectKey: body.ObjectKey,
			Body:      body.Body,
			Rev:       body.Rev,
			Exists:    body.Exists,
		}
		size := wire.SizeVT()
		want := 1 + varintLen(uint64(size)) + size
		if got := world.ObjectBodyEncodedSizeForTest(body); got != want {
			t.Fatalf("encoded size for %q = %d, want %d", body.ObjectKey, got, want)
		}
	}
}

func varintLen(v uint64) int {
	n := 1
	for v >= 128 {
		v >>= 7
		n++
	}
	return n
}
