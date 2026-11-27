// Package device_policy stores and watches daemon-local Device policy.
package device_policy

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
)

// PolicyStore stores the current daemon-local Device policy and broadcasts
// reloads.
//
// The zero value holds the empty policy, which grants nothing, until Load
// records its state root. The process creates one store before it parses its
// flags, so the controllers it composes and the serve command share it. Serve
// loads it before it starts the bus.
type PolicyStore struct {
	// bcast guards stateRoot and policy and wakes watchers after every load.
	bcast     broadcast.Broadcast
	stateRoot string
	policy    *DevicePolicy
}

// NewPolicyStore constructs a PolicyStore by loading the policy file once.
func NewPolicyStore(stateRoot string) (*PolicyStore, error) {
	s := &PolicyStore{}
	if err := s.Load(stateRoot); err != nil {
		return nil, err
	}
	return s, nil
}

// Load reads the policy file under stateRoot, makes it the current policy,
// and records stateRoot for later reloads.
func (s *PolicyStore) Load(stateRoot string) error {
	policy, err := ReadFile(stateRoot)
	if err != nil {
		return err
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.stateRoot = stateRoot
		s.policy = policy
		broadcast()
	})
	return nil
}

// Snapshot returns the current policy value.
func (s *PolicyStore) Snapshot() *DevicePolicy {
	if s == nil {
		return &DevicePolicy{}
	}
	var policy *DevicePolicy
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		policy = s.current()
	})
	return policy
}

// PermitsSecretPlugin reports whether the current policy lets pluginID use
// the Secrets of spaceID. It reads the policy in place, without a copy, since
// the core root asks on every Secret call a plugin makes.
func (s *PolicyStore) PermitsSecretPlugin(spaceID, pluginID string) bool {
	if s == nil {
		return false
	}
	var permits bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		permits = s.policy.PermitsSecretPlugin(spaceID, pluginID)
	})
	return permits
}

// Reload rereads the policy file Load selected and broadcasts the new current
// value.
func (s *PolicyStore) Reload() error {
	var stateRoot string
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		stateRoot = s.stateRoot
	})
	if stateRoot == "" {
		return errors.New("device policy is not loaded")
	}
	return s.Load(stateRoot)
}

// current returns a copy of the policy, empty before the first load. The
// caller holds bcast.
func (s *PolicyStore) current() *DevicePolicy {
	if s.policy == nil {
		return &DevicePolicy{}
	}
	return s.policy.CloneVT()
}

// WaitChange returns the current policy when it differs from last.
func (s *PolicyStore) WaitChange(ctx context.Context, last *DevicePolicy) (*DevicePolicy, error) {
	if s == nil {
		return &DevicePolicy{}, nil
	}
	for {
		var policy *DevicePolicy
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			policy = s.current()
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
