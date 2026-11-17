package publisher

import (
	"context"

	"github.com/pkg/errors"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// reclaim rewrites the destination's packs that are at least half dead once
// the checkpoint whose World is head has been posted. blocks is the complete
// closure of head: every ref of a decoded block is in blocks. read loads a
// closure block's data by hash. The pass stops when the destination head
// moves away from head. Returns the dead value bytes dropped.
func reclaim(ctx context.Context, opts cdn_publish.Options, head *bucket.ObjectRef, blocks []packedBlock, read cdn_publish.ReadBlock) (uint64, error) {
	// Index the live blocks and require the closure to hold every stored ref
	// of a decoded block, since a missed subtree would read as dead.
	live := make([]*hash.Hash, len(blocks))
	keys := make(map[string]struct{}, len(blocks))
	for i, entry := range blocks {
		live[i] = entry.ref.GetHash()
		keys[string(packfile.BlockKey(entry.ref.GetHash()))] = struct{}{}
	}
	for _, entry := range blocks {
		if !entry.decoded {
			continue
		}
		for _, ref := range entry.refs {
			if ref.GetEmpty() {
				continue
			}
			if _, ok := keys[string(packfile.BlockKey(ref.GetHash()))]; !ok {
				return 0, errors.Errorf("release closure lacks %s referenced by %s", ref.MarshalString(), entry.ref.MarshalString())
			}
		}
	}

	// The checkpoint holds head without its bucket binding.
	want := head.CloneVT()
	want.BucketId = ""

	// Replace the mostly dead packs while the destination head stays at head.
	check := func(ctx context.Context) error {
		current, err := cdn_publish.FetchSourceHeadRef(ctx, opts.Client, opts.DstSpaceID)
		if err != nil {
			return errors.Wrap(err, "read destination head")
		}
		if !current.EqualVT(want) {
			return errors.New("destination head moved during reclaim")
		}
		return nil
	}
	return cdn_publish.Reclaim(ctx, opts, live, read, check)
}
