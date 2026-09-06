package bldr_manifest_materializer

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	block "github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	bucket "github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestMaterializeManifestSourceRpcService copies a gzip-encoded manifest
// across two independent buses and volumes: the source blocks are served only
// on the source bus through a BlockStore RPC service, and the destination
// volume never holds the source blocks before the copy.
func TestMaterializeManifestSourceRpcService(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Two independent buses and volumes with distinct peer-derived IDs.
	srcTB, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(srcTB.Release)
	destTB, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(destTB.Release)

	applyTestBucketConfigs(t, ctx, srcTB)
	applyTestBucketConfigs(t, ctx, destTB)
	if srcTB.Volume.GetID() == destTB.Volume.GetID() {
		t.Fatal("test setup: volume IDs must differ between buses")
	}

	// Build the gzip-encoded source manifest on the source bus.
	srcTransformConf, destTransformConf := buildTestTransformConfs(t)
	srcCursor := buildTestSourceCursor(t, ctx, srcTB, srcTransformConf)
	defer srcCursor.Release()
	srcRef, dirRef, srcManifest := buildTestSourceManifest(t, ctx, srcCursor)

	// Serve the source bucket's raw encoded blocks over a BlockStore RPC
	// service on the source bus pipe.
	hostMux := srpc.NewMux()
	if err := hostMux.Register(block_rpc.NewSRPCBlockStoreHandler(
		block_rpc_server.NewBlockStore(srcCursor.GetBucket()),
		"fixture-source",
	)); err != nil {
		t.Fatal(err.Error())
	}
	hostClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(hostMux)))

	// Bridge the source pipe into the destination bus under the plugin-host
	// prefix, modeling the existing plugin-host service stripping.
	ctrl := bifrost_rpc.NewClientController(
		le,
		destTB.Bus,
		controller.NewInfo("test-source", controller.MustParseVersion("0.0.1"), "source block bridge"),
		hostClient,
		[]string{"plugin-host/"},
	)
	rel, err := destTB.Bus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	// The destination bucket must not already hold the source root block:
	// the copy must read every block over the RPC service.
	rawDest, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		destTB.Bus,
		le,
		destTB.StepFactorySet,
		"materializer-rpc-dest",
		destTB.Volume.GetID(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rawDest.Release()
	rootRef := srcRef.GetRootRef()
	if _, exists, err := rawDest.GetBucket().GetBlock(ctx, rootRef); err != nil {
		t.Fatal(err.Error())
	} else if exists {
		t.Fatal("test setup: destination volume already holds the source root block")
	}

	// Serve the production Materializer on the destination bus only.
	mux := srpc.NewMux()
	if err := SRPCRegisterMaterializer(mux, NewMaterializer(le, destTB.Bus, destTB.StepFactorySet)); err != nil {
		t.Fatal(err.Error())
	}
	client := NewSRPCMaterializerClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// Normalize the source ref: resolved inline transform, no conf ref.
	srcRefWithOpArgs := srcCursor.GetRefWithOpArgs()
	srcRefWithOpArgs.TransformConf = srcCursor.GetTransformConf().CloneVT()
	srcRefWithOpArgs.TransformConfRef = nil

	strm, err := client.MaterializeManifest(ctx, &MaterializeManifestRequest{
		Source:                   srcRefWithOpArgs,
		SourceServiceId:          "plugin-host/fixture-source",
		Destination:              &bucket.BucketOpArgs{BucketId: "materializer-rpc-dest", VolumeId: destTB.Volume.GetID()},
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
	gotStats := terminal.GetStats()
	if gotStats.GetBlocksSeen() != 2 || gotStats.GetBlocksCopied() != 2 {
		t.Fatalf("terminal stats = %#v, want blocks_seen = 2 and blocks_copied = 2", gotStats)
	}

	// The copied root must match the source root hash, sit in the RPC dest
	// bucket, and carry the source (gzip) transform: the engine preserves the
	// source encoded bytes at rest.
	gotRef := terminal.GetCopiedRef()
	if !gotRef.GetRootRef().EqualsRef(rootRef) {
		t.Fatalf("copied root %s != source root %s", gotRef.GetRootRef().MarshalString(), rootRef.MarshalString())
	}
	if gotRef.GetBucketId() != "materializer-rpc-dest" {
		t.Fatalf("copied bucket id = %q, want %q", gotRef.GetBucketId(), "materializer-rpc-dest")
	}
	if !gotRef.GetTransformConf().EqualVT(srcTransformConf) {
		t.Fatalf("copied transform conf = %v, want source transform %v", gotRef.GetTransformConf(), srcTransformConf)
	}

	// The raw stored bytes for the manifest root and the nested directory
	// node must be identical to the source bucket's at-rest bytes.
	for _, ref := range []*struct {
		name string
		ref  *block.BlockRef
	}{{"manifest root", rootRef}, {"directory node", dirRef}} {
		wantData, wantExists, err := srcCursor.GetBucket().GetBlock(ctx, ref.ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !wantExists {
			t.Fatalf("test setup: source bucket missing %s", ref.name)
		}
		gotData, gotExists, err := rawDest.GetBucket().GetBlock(ctx, ref.ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !gotExists {
			t.Fatalf("copied %s missing from destination bucket", ref.name)
		}
		if !bytes.Equal(gotData, wantData) {
			t.Fatalf("copied %s raw bytes != source raw bytes", ref.name)
		}
	}

	// Decode the copied manifest through the copied ref and compare it with
	// the source manifest.
	gotCursor, err := rawDest.FollowRef(ctx, gotRef)
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
