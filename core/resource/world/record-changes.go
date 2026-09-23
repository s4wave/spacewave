package resource_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
)

// CompareObjectRecords compares immutable KV roots within the supplied snapshot.
// Missing prior roots and bounded comparisons request full reevaluation.
func (r *WorldStateResource) CompareObjectRecords(ctx context.Context, req *sdk_world.CompareObjectRecordsRequest) (*sdk_world.CompareObjectRecordsResponse, error) {
	if len(req.GetBases()) > 256 {
		return nil, errors.New("at most 256 watched objects are allowed")
	}
	result := &sdk_world.CompareObjectRecordsResponse{}
	for _, base := range req.GetBases() {
		change := &sdk_world.ObjectRecordChanges{ObjectKey: base.GetObjectKey(), Unknown: true}
		result.Changes = append(result.Changes, change)
		object, found, err := r.ws.GetObject(ctx, base.GetObjectKey())
		if err != nil {
			world.ReleaseObjectState(object)
			return nil, err
		}
		if !found || base.GetRootRef().GetEmpty() {
			world.ReleaseObjectState(object)
			continue
		}
		current, _, err := object.GetRootRef(ctx)
		world.ReleaseObjectState(object)
		if err != nil {
			return nil, err
		}
		if current.EqualsRef(base.GetRootRef()) {
			change.Unknown = false
			continue
		}
		if current.GetEmpty() {
			continue
		}
		err = r.ws.AccessWorldState(ctx, base.GetRootRef(), func(before *bucket_lookup.Cursor) error {
			return r.ws.AccessWorldState(ctx, current, func(after *bucket_lookup.Cursor) error {
				_, left := before.BuildTransaction(nil)
				_, right := after.BuildTransaction(nil)
				keys, complete, err := kvtx_block.ChangedKeys(ctx, left, right, 16384)
				if err != nil {
					return err
				}
				change.Keys, change.Unknown = keys, !complete
				return nil
			})
		})
		if err != nil && !errors.Is(err, block.ErrNotFound) {
			return nil, err
		}
	}
	return result, nil
}
