package bldr_manifest_materializer

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/sirupsen/logrus"
)

// cancelingStream wraps the MaterializeManifest stream, canceling the handler
// context when a progress response is sent. The terminal copied_ref response
// is never sent on a canceled context, so the handler stops before completion.
type cancelingStream struct {
	SRPCMaterializer_MaterializeManifestStream
	childCtx context.Context
	cancel   context.CancelFunc
}

// Context returns the cancelable handler context.
func (s *cancelingStream) Context() context.Context { return s.childCtx }

// Send cancels the handler context before forwarding a progress response.
func (s *cancelingStream) Send(resp *MaterializeManifestResponse) error {
	if resp.GetCopiedRef() == nil {
		s.cancel()
	}
	return s.SRPCMaterializer_MaterializeManifestStream.Send(resp)
}

// cancelingMaterializer serves a Materializer with a cancelable child context
// per call, canceled by the stream wrapper on the first progress response.
type cancelingMaterializer struct {
	*Materializer
}

// MaterializeManifest runs the copy with a wrapped stream.
func (m *cancelingMaterializer) MaterializeManifest(
	req *MaterializeManifestRequest,
	strm SRPCMaterializer_MaterializeManifestStream,
) error {
	childCtx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	return m.Materializer.MaterializeManifest(req, &cancelingStream{
		SRPCMaterializer_MaterializeManifestStream: strm,
		childCtx: childCtx,
		cancel:   cancel,
	})
}

// buildTestDirManifest stores 70 unique empty directory children and one root
// directory node linking them with sorted dirents, then the manifest root
// referencing the directory node.
func buildTestDirManifest(
	t *testing.T,
	ctx context.Context,
	srcCursor *bucket_lookup.Cursor,
) *bucket.ObjectRef {
	t.Helper()

	// Store 70 unique empty directory children with distinct permissions.
	dirents := make([]*unixfs_block.Dirent, 0, 70)
	for i := range 70 {
		nodeData, err := (&unixfs_block.FSNode{
			NodeType:    unixfs_block.NodeType_NodeType_DIRECTORY,
			Permissions: uint32(i + 1),
		}).MarshalVT()
		if err != nil {
			t.Fatal(err.Error())
		}
		nodeRef, _, err := srcCursor.PutBlock(ctx, nodeData, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		dirents = append(dirents, &unixfs_block.Dirent{
			Name:     strconv.Itoa(100 + i),
			NodeRef:  nodeRef,
			NodeType: unixfs_block.NodeType_NodeType_DIRECTORY,
		})
	}

	// Store the root directory node referencing all children.
	rootData, err := (&unixfs_block.FSNode{
		NodeType:       unixfs_block.NodeType_NodeType_DIRECTORY,
		DirectoryEntry: dirents,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := srcCursor.PutBlock(ctx, rootData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store the manifest root referencing the directory node.
	srcManifest := &bldr_manifest.Manifest{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: "test-manifest-cancel",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        1,
		},
		Entrypoint: "entrypoint",
		DistFsRef:  rootRef,
	}
	manifestData, err := srcManifest.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	manifestRef, _, err := srcCursor.PutBlock(ctx, manifestData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(manifestRef)
	srcRef := srcCursor.GetRef()
	if srcRef.GetRootRef().GetEmpty() {
		t.Fatal("test setup: source ref is empty")
	}
	return srcRef
}

// TestMaterializeManifestCancelActiveCopy cancels the handler context while a
// large copy is actively traversing and asserts the RPC ends with the canceled
// error and never emits a copied root.
func TestMaterializeManifestCancelActiveCopy(t *testing.T) {
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

	srcRef := buildTestDirManifest(t, ctx, srcCursor)

	// Serve the Materializer over an in-memory srpc pipe with the canceling
	// stream wrapper.
	mux := srpc.NewMux()
	if err := SRPCRegisterMaterializer(mux, &cancelingMaterializer{
		Materializer: NewMaterializer(le, tb.Bus, tb.StepFactorySet),
	}); err != nil {
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))

	client := NewSRPCMaterializerClient(srpcClient)
	strm, err := client.MaterializeManifest(ctx, &MaterializeManifestRequest{
		Source:      srcRef,
		Destination: &bucket.BucketOpArgs{BucketId: "materializer-rpc-dest", VolumeId: tb.Volume.GetID()},
		Concurrency: 1,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Drain the stream: at least one progress response with at least 64
	// blocks seen, no copied root, and a terminal canceled error.
	progressCount := 0
	maxBlocksSeen := int64(0)
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
			continue
		}
		if stats := resp.GetStats(); stats != nil {
			progressCount++
			if stats.GetBlocksSeen() > maxBlocksSeen {
				maxBlocksSeen = stats.GetBlocksSeen()
			}
		}
	}
	if streamErr == nil {
		t.Fatal("stream ended without error, want canceled error")
	}
	if errors.Is(streamErr, io.EOF) {
		t.Fatalf("stream ended with EOF, want canceled error: %v", streamErr)
	}
	if !strings.Contains(streamErr.Error(), "canceled") {
		t.Fatalf("stream error = %v, want canceled", streamErr)
	}
	if progressCount == 0 {
		t.Fatal("no progress responses received before cancellation")
	}
	if maxBlocksSeen < 64 {
		t.Fatalf("max blocks seen = %d, want >= 64", maxBlocksSeen)
	}
	if copiedCount != 0 {
		t.Fatalf("copied root count = %d, want 0", copiedCount)
	}
}
