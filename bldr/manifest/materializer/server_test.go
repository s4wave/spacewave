package bldr_manifest_materializer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	block "github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_lz4 "github.com/s4wave/spacewave/db/block/transform/lz4"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/sirupsen/logrus"
)

// buildTestTransformConfs builds the source (gzip) and destination (lz4)
// transform configurations.
func buildTestTransformConfs(t *testing.T) (*block_transform.Config, *block_transform.Config) {
	t.Helper()

	srcTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	destTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_lz4.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return srcTransformConf, destTransformConf
}

// applyTestBucketConfigs applies the source, direct-copy, and RPC-copy bucket
// configs to the testbed volume.
func applyTestBucketConfigs(t *testing.T, ctx context.Context, tb *testbed.Testbed) {
	t.Helper()

	const srcBucketID = "materializer-source"
	const directDestBucketID = "materializer-direct-dest"
	const rpcDestBucketID = "materializer-rpc-dest"
	for _, bucketID := range []string{srcBucketID, directDestBucketID, rpcDestBucketID} {
		if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
			Id:  bucketID,
			Rev: 1,
		}); err != nil {
			t.Fatal(err.Error())
		}
	}
}

// buildTestSourceCursor builds the source cursor with the gzip transform.
func buildTestSourceCursor(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	srcTransformConf *block_transform.Config,
) *bucket_lookup.Cursor {
	t.Helper()

	srcCursor, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		"materializer-source",
		tb.Volume.GetID(),
		srcTransformConf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return srcCursor
}

// buildTestSourceManifest builds a manifest in the source bucket whose dist
// and assets refs point at one shared empty directory node, so the copy sees
// exactly two unique blocks: the directory node and the manifest root.
func buildTestSourceManifest(
	t *testing.T,
	ctx context.Context,
	srcCursor *bucket_lookup.Cursor,
) (*bucket.ObjectRef, *block.BlockRef, *bldr_manifest.Manifest) {
	t.Helper()

	// Store one empty directory node: a valid FSNode with no children.
	nodeData, err := (&unixfs_block.FSNode{
		NodeType: unixfs_block.NodeType_NodeType_DIRECTORY,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	dirRef, _, err := srcCursor.PutBlock(ctx, nodeData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write the manifest root referencing the shared directory twice.
	srcManifest := &bldr_manifest.Manifest{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: "test-manifest",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        1,
		},
		Entrypoint:  "entrypoint",
		DistFsRef:   dirRef,
		AssetsFsRef: dirRef,
	}
	btx, bcs := srcCursor.BuildTransaction(nil)
	bcs.SetBlock(srcManifest, true)
	manifestRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(manifestRef)
	srcRef := srcCursor.GetRef()
	if srcRef.GetRootRef().GetEmpty() {
		t.Fatal("test setup: source ref is empty")
	}
	return srcRef, dirRef, srcManifest
}

// copyDirectWithEngine copies the source manifest to the direct-dest bucket
// with the existing copy engine, returning the expected root reference and the
// raw stored bytes of the shared directory node in the direct-dest bucket.
func copyDirectWithEngine(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	srcRef *bucket.ObjectRef,
	dirRef *block.BlockRef,
	destTransformConf *block_transform.Config,
) (*bucket.ObjectRef, []byte) {
	t.Helper()

	directDest, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		"materializer-direct-dest",
		tb.Volume.GetID(),
		destTransformConf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer directDest.Release()
	directSrc, err := bldr_manifest_world.FollowObjectRefReadOnly(ctx, directDest, srcRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer directSrc.Release()
	expectedRef, _, err := bucket_lookup.CopyObjectToBucketWithStats(
		ctx,
		directDest,
		directSrc,
		bldr_manifest.NewManifestBlock,
		-1,
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	gotNodeData, exists, err := directDest.GetBucket().GetBlock(ctx, dirRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("direct copy: directory node missing from dest bucket")
	}
	return expectedRef, gotNodeData
}

// TestMaterializeManifestRPCCopy copies a nested manifest through the typed
// RPC service across two in-memory buckets and compares the result with the
// existing copy engine.
func TestMaterializeManifestRPCCopy(t *testing.T) {
	// t.Context is canceled at test cleanup, releasing all bus resources.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	srcTransformConf, destTransformConf := buildTestTransformConfs(t)
	applyTestBucketConfigs(t, ctx, tb)

	srcCursor := buildTestSourceCursor(t, ctx, tb, srcTransformConf)
	defer srcCursor.Release()

	srcRef, dirRef, srcManifest := buildTestSourceManifest(t, ctx, srcCursor)

	// Copy directly with the existing engine to compute the expected root and
	// the expected raw stored bytes.
	expectedRef, wantNodeData := copyDirectWithEngine(t, ctx, tb, srcRef, dirRef, destTransformConf)

	// Serve the Materializer over an in-memory srpc pipe.
	mux := srpc.NewMux()
	if err := SRPCRegisterMaterializer(mux, NewMaterializer(le, tb.Bus, tb.StepFactorySet)); err != nil {
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))

	// Call the RPC with the source ref and the rpc-dest bucket.
	client := NewSRPCMaterializerClient(srpcClient)
	strm, err := client.MaterializeManifest(ctx, &MaterializeManifestRequest{
		Source:                   srcRef,
		Destination:              &bucket.BucketOpArgs{BucketId: "materializer-rpc-dest", VolumeId: tb.Volume.GetID()},
		DestinationTransformConf: destTransformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Drain the stream: progress responses then exactly one terminal copied_ref.
	var terminal *MaterializeManifestResponse
	terminalCount := 0
	for {
		resp, err := strm.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err.Error())
		}
		if resp.GetCopiedRef() != nil {
			terminalCount++
			terminal = resp
		}
	}
	if terminalCount != 1 {
		t.Fatalf("terminal copied_ref count = %d, want 1", terminalCount)
	}
	if terminal.GetStats() == nil {
		t.Fatal("terminal response missing stats")
	}

	// Verify the terminal stats: the shared dist/assets node is deduped.
	gotStats := terminal.GetStats()
	if gotStats.GetBlocksSeen() != 2 || gotStats.GetBlocksCopied() != 2 {
		t.Fatalf("terminal stats = %#v, want blocks_seen = 2 and blocks_copied = 2", gotStats)
	}

	// Compare the RPC result with the direct copy engine result: root and
	// transform fields must match, and the bucket must be the RPC dest bucket.
	gotRef := terminal.GetCopiedRef()
	if !gotRef.GetRootRef().EqualsRef(expectedRef.GetRootRef()) {
		t.Fatalf("copied root %s != expected root %s", gotRef.GetRootRef().MarshalString(), expectedRef.GetRootRef().MarshalString())
	}
	if gotRef.GetBucketId() != "materializer-rpc-dest" {
		t.Fatalf("copied bucket id = %q, want %q", gotRef.GetBucketId(), "materializer-rpc-dest")
	}
	if !gotRef.GetTransformConf().EqualVT(expectedRef.GetTransformConf()) {
		t.Fatalf("copied transform conf = %v, want %v", gotRef.GetTransformConf(), expectedRef.GetTransformConf())
	}

	// Verify the nested FSNode raw encoded bytes were copied to the dest
	// bucket: blocks are stored with the source encoding, so the stored node
	// bytes must be gzip-encoded and identical to the direct-copy result.
	rpcDest, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		"materializer-rpc-dest",
		tb.Volume.GetID(),
		destTransformConf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rpcDest.Release()
	gotNodeData, exists, err := rpcDest.GetBucket().GetBlock(ctx, dirRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("rpc copy: directory node missing from dest bucket")
	}
	if !bytes.Equal(gotNodeData, wantNodeData) {
		t.Fatal("rpc copy stored bytes != direct copy stored bytes for directory node")
	}

	// Decode the manifest at the copied root through the dest transform and
	// compare it with the source manifest.
	gotCursor, err := rpcDest.FollowRef(ctx, gotRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer gotCursor.Release()
	_, bcs := gotCursor.BuildTransaction(nil)
	gotManifest, err := bldr_manifest.UnmarshalManifest(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !gotManifest.EqualVT(srcManifest) {
		t.Fatalf("decoded manifest = %v, want %v", gotManifest, srcManifest)
	}
}
