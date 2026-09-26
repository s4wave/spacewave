package bldr_manifest_pack

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestManifestPackStartsAtBundleRoot verifies physical layout and deterministic
// output through the real manifest pack writer, including a filesystem tree.
func TestManifestPackStartsAtBundleRoot(t *testing.T) {
	ctx := t.Context()
	ws := newTestWorld(t, ctx, logrus.NewEntry(logrus.New()))
	tuple := testManifestPackTuple()
	manifest := storeTestManifest(t, ctx, ws, tuple, false, true)
	_, bundle, err := StoreManifestBundle(ctx, ws, peer.ID("sender"), tuple, manifest, timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}

	var first []byte
	for range 4 {
		var out bytes.Buffer
		_, _, err := PackManifestBundle(ctx, ws, "locality", bundle, &out)
		if err != nil {
			t.Fatal(err)
		}
		if first != nil && !bytes.Equal(first, out.Bytes()) {
			t.Fatal("unchanged manifest produced a different physical pack order")
		}
		first = bytes.Clone(out.Bytes())
	}

	reader, err := kvfile.BuildReader(bytes.NewReader(first), uint64(len(first)))
	if err != nil {
		t.Fatal(err)
	}
	pos, _, _, _, err := reader.GetValuePosition(packfile.BlockKey(bundle.GetRootRef().GetHash()))
	if err != nil {
		t.Fatal(err)
	}
	if pos != 0 {
		t.Fatalf("bundle root starts at %d, want the first payload at 0", pos)
	}
}
