package bldr_manifest_materializer

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	block "github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestMaterializeManifestMissingChildBlock copies a manifest whose dist ref
// points at a block that was never stored and asserts the RPC fails with the
// missing-block error and never emits a copied root.
func TestMaterializeManifestMissingChildBlock(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	applyTestBucketConfigs(t, ctx, tb)
	srcCursor := buildTestSourceCursor(t, ctx, tb, nil)
	defer srcCursor.Release()

	// Reference a block that is never stored in the source bucket.
	missingRef, err := block.BuildBlockRef([]byte("missing child"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store the manifest root referencing the missing block.
	srcManifest := &bldr_manifest.Manifest{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: "test-manifest-missing",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        1,
		},
		Entrypoint: "entrypoint",
		DistFsRef:  missingRef,
	}
	nodeData, err := srcManifest.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	manifestRef, _, err := srcCursor.PutBlock(ctx, nodeData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(manifestRef)
	srcRef := srcCursor.GetRef()

	// Serve the Materializer over an in-memory srpc pipe.
	mux := srpc.NewMux()
	if err := SRPCRegisterMaterializer(mux, NewMaterializer(le, tb.Bus, tb.StepFactorySet)); err != nil {
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))

	client := NewSRPCMaterializerClient(srpcClient)
	strm, err := client.MaterializeManifest(ctx, &MaterializeManifestRequest{
		Source:      srcRef,
		Destination: &bucket.BucketOpArgs{BucketId: "materializer-rpc-dest", VolumeId: tb.Volume.GetID()},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Drain the stream: no copied root may arrive and the stream must end
	// with the missing-block error instead of a clean EOF.
	copiedCount := 0
	var streamErr error
	for {
		resp, err := strm.Recv()
		if err != nil {
			streamErr = err
			break
		}
		if resp.GetCopiedRef() != nil {
			copiedCount++
		}
	}
	if streamErr == nil {
		t.Fatal("stream ended without error, want missing-block error")
	}
	if errors.Is(streamErr, io.EOF) {
		t.Fatalf("stream ended with EOF, want missing-block error: %v", streamErr)
	}
	if !strings.Contains(streamErr.Error(), "block not found") {
		t.Fatalf("stream error = %v, want block not found", streamErr)
	}
	if copiedCount != 0 {
		t.Fatalf("copied root count = %d, want 0", copiedCount)
	}
}
