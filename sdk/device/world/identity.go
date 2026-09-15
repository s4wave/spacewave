package s4wave_device_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// EnsureDevice retains the canonical Device in the caller's World transaction.
// The caller must verify key possession and permission to pair this peer before
// calling; an unverified request parameter is not an authenticated identity.
// Existing metadata is returned unchanged. A new Device has no setup readiness,
// liveness or capabilities until the corresponding native owner reports them.
// The caller owns commit or discard and may bind application relationships in
// the same transaction. The returned body is decoded data with no live handles.
func EnsureDevice(ctx context.Context, state world.WorldState, authenticatedPeer peer.ID, label string) (string, *s4wave_device.Device, error) {
	// Validate identity before looking up or creating a durable Device.
	if authenticatedPeer == "" {
		return "", nil, errors.New("authenticated Device peer is required")
	}
	if _, err := authenticatedPeer.ExtractPublicKey(); err != nil {
		return "", nil, errors.Wrap(err, "validate authenticated Device peer")
	}

	// Reuse the established identity without changing its native observations.
	key := EnrolledDeviceObjectKey(authenticatedPeer)
	exists, err := state.HasObject(ctx, key)
	if err != nil {
		return "", nil, err
	}
	if exists {
		if err := world_types.CheckObjectType(ctx, state, key, s4wave_device.DeviceTypeID); err != nil {
			return "", nil, err
		}
		device, err := world.LookupObjectBody[*s4wave_device.Device](ctx, state, key, s4wave_device.NewDeviceBlock)
		if err != nil {
			return "", nil, err
		}
		if device.GetPeerId() != authenticatedPeer.String() {
			return "", nil, errors.New("Device record does not match the authenticated peer")
		}
		return key, device, nil
	}

	// Pairing creates an identity, without asserting that a session has mounted.
	now := timestamppb.New(time.Now())
	device := &s4wave_device.Device{PeerId: authenticatedPeer.String(), Label: label, CreatedAt: now, UpdatedAt: now.CloneVT()}
	if err := device.Validate(); err != nil {
		return "", nil, err
	}
	object, _, err := world.CreateWorldObject(ctx, state, key, func(cursor *block.Cursor) error {
		cursor.SetBlock(device, true)
		return nil
	})
	world.ReleaseObjectState(object)
	if err != nil {
		return "", nil, err
	}
	if err := world_types.SetObjectType(ctx, state, key, s4wave_device.DeviceTypeID); err != nil {
		return "", nil, err
	}
	return key, device, nil
}
