// Package device_policy stores and watches daemon-local Device policy.
package device_policy

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

// PolicyStore stores the current daemon-local Device policy and broadcasts reloads.
type PolicyStore struct {
	stateRoot string

	// bcast guards policy and wakes watchers after every reload.
	bcast  broadcast.Broadcast
	policy *DevicePolicy
}

// NewPolicyStore constructs a PolicyStore by loading the policy file once.
func NewPolicyStore(stateRoot string) (*PolicyStore, error) {
	policy, err := ReadFile(stateRoot)
	if err != nil {
		return nil, err
	}
	return &PolicyStore{stateRoot: stateRoot, policy: policy.CloneVT()}, nil
}

// Snapshot returns the current policy value.
func (s *PolicyStore) Snapshot() *DevicePolicy {
	if s == nil {
		return (&DevicePolicy{}).CloneVT()
	}
	var policy *DevicePolicy
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		policy = s.policy.CloneVT()
	})
	return policy
}

// Reload loads the policy file and broadcasts the new current value.
func (s *PolicyStore) Reload() error {
	policy, err := ReadFile(s.stateRoot)
	if err != nil {
		return err
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.policy = policy.CloneVT()
		broadcast()
	})
	return nil
}

// WaitChange returns the current policy when it differs from last.
func (s *PolicyStore) WaitChange(ctx context.Context, last *DevicePolicy) (*DevicePolicy, error) {
	if s == nil {
		return (&DevicePolicy{}).CloneVT(), nil
	}
	for {
		var policy *DevicePolicy
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			policy = s.policy.CloneVT()
			waitCh = getWaitCh()
		})
		if last == nil || !policy.EqualVT(last) {
			return policy, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
	}
}
