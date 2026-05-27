package resource_session

import (
	"context"

	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchStorageStats streams provider-owned session storage usage snapshots.
func (r *SessionResource) WatchStorageStats(
	req *s4wave_session.WatchStorageStatsRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchStorageStatsStream,
) error {
	ctx := strm.Context()
	var prev *s4wave_session.WatchStorageStatsResponse
	for {
		resp, waitCh, err := r.buildStorageStatsSnapshot(ctx)
		if err != nil {
			return err
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp.CloneVT()
		}
		if err := waitStorageStats(ctx, waitCh); err != nil {
			return err
		}
	}
}

func (r *SessionResource) buildStorageStatsSnapshot(
	ctx context.Context,
) (*s4wave_session.WatchStorageStatsResponse, <-chan struct{}, error) {
	acc, ok := r.session.GetProviderAccount().(provider.StorageStatsWatchProvider)
	if !ok {
		return &s4wave_session.WatchStorageStatsResponse{}, nil, nil
	}
	stats, waitCh, err := acc.GetStorageStatsSnapshotWithWait(ctx)
	if err != nil {
		return nil, nil, err
	}
	return storageStatsToProto(stats), waitCh, nil
}

func storageStatsToProto(stats *volume.StorageStats) *s4wave_session.WatchStorageStatsResponse {
	resp := &s4wave_session.WatchStorageStatsResponse{Supported: true}
	if stats == nil {
		return resp
	}
	resp.TotalBytes = stats.GetTotalBytes()
	resp.BlockCount = stats.GetBlockCount()
	return resp
}

func waitStorageStats(ctx context.Context, waitCh <-chan struct{}) error {
	if waitCh == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-waitCh:
		return nil
	}
}
