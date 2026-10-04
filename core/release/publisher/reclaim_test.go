package publisher

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
)

// reclaimSpace is the destination Space of the reclaim tests.
const reclaimSpace = "reclaim-space"

// reclaimClient is a destination catalog with a fixed head.
type reclaimClient struct {
	// SessionClient supplies methods the reclaim must never reach.
	cdn_publish.SessionClient
	// state is the encoded destination state.
	state []byte
	// entries is the destination catalog.
	entries []*packfile.PackfileEntry
	// pushed are the keys of each ordinary pack upload.
	pushed [][]string
	// replaced are the replaced pack IDs of each replacement.
	replaced [][]string
	// replacedKeys are the keys of each replacement pack.
	replacedKeys [][]string
}

// GetSOState returns the destination state.
func (c *reclaimClient) GetSOState(context.Context, string, uint64, spacewave_provider.SeedReason) ([]byte, error) {
	return c.state, nil
}

// SyncPull returns the destination catalog.
func (c *reclaimClient) SyncPull(context.Context, string, uint64) (*packfile.PullResponse, error) {
	return &packfile.PullResponse{Entries: c.entries}, nil
}

// SyncPushData records the keys of an ordinary upload.
func (c *reclaimClient) SyncPushData(_ context.Context, _, _ string, _ int, data, _, _ []byte, _ uint32) error {
	c.pushed = append(c.pushed, packKeys(data))
	return nil
}

// SyncReplaceData records a replacement.
func (c *reclaimClient) SyncReplaceData(_ context.Context, _, _ string, _ int, data, _, _ []byte, _ uint32, replaced []string) error {
	c.replaced = append(c.replaced, slices.Clone(replaced))
	c.replacedKeys = append(c.replacedKeys, packKeys(data))
	return nil
}

// TestReclaimReplacesMostlyDeadPacks supersedes a mostly dead pack and a dead
// pack with one pack of their live block, then borrows a kept pack when a
// group has no live block, and stops when the head moved.
func TestReclaimReplacesMostlyDeadPacks(t *testing.T) {
	// Build a two-block release closure and two dead blocks.
	leaf := newPackedBlock(t, []byte("live leaf"))
	root := newPackedBlock(t, []byte("live root"), leaf.ref)
	deadA := newPackedBlock(t, bytes.Repeat([]byte("a"), 4096))
	deadB := newPackedBlock(t, bytes.Repeat([]byte("b"), 4096))
	blocks := []packedBlock{root, leaf}
	head := &bucket.ObjectRef{RootRef: root.ref, BucketId: "release"}

	// Publish a mostly dead pack, a dead pack and a live pack.
	packs := map[string][]byte{}
	mixed := newPack(t, packs, deadA, leaf)
	dead := newPack(t, packs, deadB)
	livePack := newPack(t, packs, root)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], ".kvf")
		http.ServeContent(w, r, id, time.Time{}, bytes.NewReader(packs[id]))
	}))
	t.Cleanup(server.Close)
	client := &reclaimClient{state: reclaimState(t, head), entries: []*packfile.PackfileEntry{mixed, dead, livePack}}
	opts := cdn_publish.Options{Client: client, DstSpaceID: reclaimSpace, CdnBaseURL: server.URL}

	// The live leaf supersedes both dead-heavy packs in one replacement.
	dropped, err := reclaim(t.Context(), opts, head, blocks)
	if err != nil {
		t.Fatal(err)
	}
	leafKey := string(packfile.BlockKey(leaf.ref.GetHash()))
	if len(client.pushed) != 0 || len(client.replaced) != 1 ||
		!slices.Equal(client.replaced[0], []string{mixed.GetId(), dead.GetId()}) ||
		!slices.Equal(client.replacedKeys[0], []string{leafKey}) {
		t.Fatalf("pushed=%v replaced=%v keys=%v", client.pushed, client.replaced, client.replacedKeys)
	}
	if dropped < 8192 {
		t.Fatalf("dropped %d bytes", dropped)
	}

	// A group of dead packs borrows the smallest kept pack.
	client.entries = []*packfile.PackfileEntry{dead, livePack}
	client.replaced, client.replacedKeys = nil, nil
	if _, err := reclaim(t.Context(), opts, head, blocks); err != nil {
		t.Fatal(err)
	}
	rootKey := string(packfile.BlockKey(root.ref.GetHash()))
	if len(client.replaced) != 1 ||
		!slices.Equal(client.replaced[0], []string{dead.GetId(), livePack.GetId()}) ||
		!slices.Equal(client.replacedKeys[0], []string{rootKey}) {
		t.Fatalf("borrow: replaced=%v keys=%v", client.replaced, client.replacedKeys)
	}

	// A moved head stops the pass before any replacement.
	client.state = reclaimState(t, &bucket.ObjectRef{RootRef: leaf.ref})
	client.replaced = nil
	if _, err := reclaim(t.Context(), opts, head, blocks); err == nil || len(client.replaced) != 0 {
		t.Fatalf("moved head: err=%v replaced=%v", err, client.replaced)
	}

	// A closure missing a referenced block judges nothing.
	if _, err := reclaim(t.Context(), opts, head, []packedBlock{root}); err == nil {
		t.Fatal("reclaimed against an open closure")
	}
}

// newPackedBlock stores data with refs as a release block.
func newPackedBlock(t *testing.T, data []byte, refs ...*block.BlockRef) packedBlock {
	t.Helper()
	h, err := hash.Sum(hash.RecommendedHashType, data)
	if err != nil {
		t.Fatal(err)
	}
	return packedBlock{ref: block.NewBlockRef(h), stored: &block.StoredBlock{Data: data, Refs: refs, RefsKnown: true}}
}

// newPack writes blocks as one destination pack into packs.
func newPack(t *testing.T, packs map[string][]byte, blocks ...packedBlock) *packfile.PackfileEntry {
	// Pack the blocks in order and serve the single resulting pack.
	t.Helper()
	var out *packfile.PackfileEntry
	_, err := delta.EmitDeltaChunks(t.Context(), reclaimSpace, func() (*hash.Hash, *block.StoredBlock, error) {
		if len(blocks) == 0 {
			return nil, nil, nil
		}
		next := blocks[0]
		blocks = blocks[1:]
		return next.ref.GetHash(), next.stored, nil
	}, delta.DefaultMaxChunkBytes, func(_ context.Context, _ int, entry *packfile.PackfileEntry, data []byte) error {
		out = entry.CloneVT()
		out.SizeBytes = uint64(len(data))
		packs[out.GetId()] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// packKeys returns the index keys of a pack.
func packKeys(data []byte) []string {
	// Open the pack index.
	rd, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return nil
	}

	// Collect its keys in order.
	var keys []string
	_ = rd.ScanPrefixKeys(nil, func(key []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	return keys
}

// reclaimState encodes a destination state whose World is head.
func reclaimState(t *testing.T, head *bucket.ObjectRef) []byte {
	// Sign a genesis checkpoint of head with a disposable key.
	t.Helper()
	stateData, err := cdn_publish.EncodeHeadState(head)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := sobject.BuildGenesisSOCheckpoint(key, reclaimSpace, make([]byte, 32), stateData)
	if err != nil {
		t.Fatal(err)
	}

	// Wrap it in the state snapshot the service returns.
	out, err := (&api.SOStateMessage{
		Content: &api.SOStateMessage_Snapshot{Snapshot: &sobject.SOState{Checkpoint: checkpoint}},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return out
}
