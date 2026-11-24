package device_policy

import (
	"context"
)

// Service serves a PolicyStore as the DevicePolicyService.
type Service struct {
	// store holds the current policy and wakes watchers on reload.
	store *PolicyStore
}

// NewService constructs a DevicePolicyService over store.
func NewService(store *PolicyStore) *Service {
	return &Service{store: store}
}

// Reload rereads the policy file and wakes every watcher.
func (s *Service) Reload(_ context.Context, _ *ReloadDevicePolicyRequest) (*ReloadDevicePolicyResponse, error) {
	if err := s.store.Reload(); err != nil {
		return nil, err
	}
	return &ReloadDevicePolicyResponse{}, nil
}

// WatchDevicePolicy sends the current policy, then each changed policy, until
// the stream ends.
func (s *Service) WatchDevicePolicy(_ *WatchDevicePolicyRequest, strm SRPCDevicePolicyService_WatchDevicePolicyStream) error {
	ctx := strm.Context()
	var last *DevicePolicy
	for {
		policy, err := s.store.WaitChange(ctx, last)
		if err != nil {
			return err
		}
		if err := strm.Send(policy); err != nil {
			return err
		}
		last = policy
	}
}

// _ is a type assertion
var _ SRPCDevicePolicyServiceServer = (*Service)(nil)
