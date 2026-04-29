package publish

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
)

// FetchPackEntries reads the pack manifest for a resource-scoped block store.
func FetchPackEntries(ctx context.Context, client SessionClient, spaceID string) ([]*packfile.PackfileEntry, error) {
	pullData, err := client.SyncPull(ctx, spaceID, "")
	if err != nil {
		return nil, errors.Wrap(err, "sync pull pack manifest")
	}
	if len(pullData) == 0 {
		return nil, nil
	}
	resp := &packfile.PullResponse{}
	if err := resp.UnmarshalJSON(pullData); err != nil {
		return nil, errors.Wrap(err, "unmarshal pull response")
	}
	return resp.GetEntries(), nil
}

// DecodeHeadRef decodes a World head ref from a shared-object state snapshot.
func DecodeHeadRef(snapshot *sobject.SOState) (*bucket.ObjectRef, error) {
	root := snapshot.GetRoot()
	if root == nil || len(root.GetInner()) == 0 {
		return nil, nil
	}
	sori := &sobject.SORootInner{}
	if err := sori.UnmarshalVT(root.GetInner()); err != nil {
		return nil, errors.Wrap(err, "unmarshal SORootInner")
	}
	return DecodeHeadRefFromRootInner(sori)
}

// DecodeHeadRefFromRootInner decodes the mounted World head ref from SORootInner.
func DecodeHeadRefFromRootInner(sori *sobject.SORootInner) (*bucket.ObjectRef, error) {
	innerState := &sobject_world_engine.InnerState{}
	if err := innerState.UnmarshalVT(sori.GetStateData()); err != nil {
		return nil, errors.Wrap(err, "unmarshal inner state")
	}
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

// FetchDestinationHeadRef reads the public CDN root pointer and decodes its World head ref.
func FetchDestinationHeadRef(ctx context.Context, cdnBaseURL string, spaceID string) (*bucket.ObjectRef, error) {
	ptr, err := FetchRemoteRootPointer(ctx, cdnBaseURL, spaceID)
	if err != nil {
		return nil, err
	}
	if ptr == nil || ptr.GetRoot() == nil || len(ptr.GetRoot().GetInner()) == 0 {
		return nil, nil
	}
	sori := &sobject.SORootInner{}
	if err := sori.UnmarshalVT(ptr.GetRoot().GetInner()); err != nil {
		return nil, nil
	}
	headRef, err := DecodeHeadRefFromRootInner(sori)
	if err != nil {
		return nil, nil
	}
	return headRef, nil
}

// FetchRemoteRootSeqno fetches the CDN root pointer seqno. Missing roots return zero.
func FetchRemoteRootSeqno(ctx context.Context, cdnBaseURL, spaceID string) (uint64, error) {
	ptr, err := FetchRemoteRootPointer(ctx, cdnBaseURL, spaceID)
	if err != nil {
		return 0, err
	}
	if ptr == nil {
		return 0, nil
	}
	return ptr.GetRoot().GetInnerSeqno(), nil
}

// FetchRemoteRootPointer fetches and decodes the public CDN root pointer.
func FetchRemoteRootPointer(ctx context.Context, cdnBaseURL, spaceID string) (*alpha_cdn.CdnRootPointer, error) {
	pointerURL := cdnBaseURL + "/" + spaceID + "/root.packedmsg"
	body, status, err := FetchBytesStatus(ctx, pointerURL, MaxRootPackedmsgBytes)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status < 200 || status >= 300 {
		return nil, errors.Errorf("status %d from %s", status, pointerURL)
	}
	raw, ok := packedmsg.DecodePackedMessage(string(body))
	if !ok {
		return nil, errors.New("decode remote root.packedmsg: checksum mismatch")
	}
	pointer := &alpha_cdn.CdnRootPointer{}
	if err := pointer.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "unmarshal CdnRootPointer")
	}
	return pointer, nil
}
