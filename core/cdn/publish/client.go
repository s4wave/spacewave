package publish

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
)

// FetchPackEntries reads the pack manifest for a resource-scoped block store.
func FetchPackEntries(ctx context.Context, client SessionClient, spaceID string) ([]*packfile.PackfileEntry, error) {
	catalog, err := packfile.PullCatalog(ctx, client, spaceID)
	if err != nil {
		return nil, errors.Wrap(err, "sync pull pack manifest")
	}
	return catalog.GetEntries(), nil
}

// DecodeHeadRef decodes the World head ref of the checkpoint in a
// shared-object state snapshot. CDN Spaces hold only checkpoints with plain
// state data, so the checkpoint is their complete state.
func DecodeHeadRef(snapshot *sobject.SOState) (*bucket.ObjectRef, error) {
	return decodeCheckpointHeadRef(snapshot.GetCheckpoint())
}

// FetchDestinationHeadRef reads the public CDN root pointer and decodes the
// World head ref of its checkpoint.
func FetchDestinationHeadRef(ctx context.Context, cdnBaseURL string, spaceID string) (*bucket.ObjectRef, error) {
	ptr, err := FetchRemoteRootPointer(ctx, cdnBaseURL, spaceID)
	if err != nil {
		return nil, err
	}
	return decodeCheckpointHeadRef(ptr.GetCheckpoint())
}

// decodeCheckpointHeadRef decodes the World head ref from the plain state data
// of checkpoint. Returns nil, nil when there is no published World.
func decodeCheckpointHeadRef(checkpoint *sobject.SOCheckpoint) (*bucket.ObjectRef, error) {
	// Decode the checkpoint's World state.
	if checkpoint == nil {
		return nil, nil
	}
	inner, err := checkpoint.UnmarshalInner()
	if err != nil {
		return nil, err
	}
	innerState := &sobject_world_engine.InnerState{}
	if err := innerState.UnmarshalVT(inner.GetStateData()); err != nil {
		return nil, errors.Wrap(err, "unmarshal inner state")
	}

	// Return its head without the bucket binding.
	headRef := innerState.GetHeadRef()
	if headRef == nil || headRef.GetEmpty() {
		return nil, nil
	}
	headRef = headRef.CloneVT()
	headRef.BucketId = ""
	if err := headRef.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate head ref")
	}
	return headRef, nil
}

// FetchRemoteRootPointer fetches and decodes the public CDN root pointer.
func FetchRemoteRootPointer(ctx context.Context, cdnBaseURL, spaceID string) (*alpha_cdn.CdnRootPointer, error) {
	// Fetch the CDN root pointer bytes for the requested Space.
	pointerURL := cdnBaseURL + "/" + spaceID + "/root.packedmsg"
	body, status, err := FetchBytesStatus(ctx, pointerURL, MaxRootPackedmsgBytes)
	if err != nil {
		return nil, err
	}

	// Handle missing or unsuccessful CDN root responses before decoding bytes.
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status < 200 || status >= 300 {
		return nil, errors.Errorf("status %d from %s", status, pointerURL)
	}

	// Verify the packed CDN root pointer before unmarshaling its record.
	raw, ok := packedmsg.DecodePackedMessage(string(body))
	if !ok {
		return nil, errors.New("decode remote root.packedmsg: checksum mismatch")
	}

	// Unmarshal the verified bytes into the CDN root-pointer record.
	pointer := &alpha_cdn.CdnRootPointer{}
	if err := pointer.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "unmarshal CdnRootPointer")
	}
	return pointer, nil
}
