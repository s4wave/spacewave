package s4wave_device_world

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// EnrolledDeviceObjectKey preserves the Device setup key derived from the
// authenticated peer's printable identity. The identifier is a 128-bit hash.
func EnrolledDeviceObjectKey(id peer.ID) string {
	sum := sha256.Sum256([]byte(id.String()))
	return "devices/" + hex.EncodeToString(sum[:16])
}

// EnsureEnrolledDevice projects a successfully mounted DEVICE session into its
// writable World. The caller must obtain authenticatedPeer from that mount,
// after admission, never from a browser request or configured object key.
// Existing metadata and capabilities are retained; only setup readiness changes.
// Repeated ready projections do not create a World revision.
func EnsureEnrolledDevice(ctx context.Context, engine world.Engine, authenticatedPeer peer.ID, label string) (string, error) {
	// Reject incomplete mount identity before acquiring a write transaction.
	if engine == nil || authenticatedPeer == "" {
		return "", errors.New("mounted World and authenticated Device peer are required")
	}
	if _, err := authenticatedPeer.ExtractPublicKey(); err != nil {
		return "", errors.Wrap(err, "validate authenticated Device peer")
	}
	if strings.TrimSpace(label) == "" {
		return "", errors.New("Device label is required")
	}

	// Resolve the established Device key under the World's transaction lock.
	key := EnrolledDeviceObjectKey(authenticatedPeer)
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return "", err
	}
	defer tx.Discard()
	exists, err := tx.HasObject(ctx, key)
	if err != nil {
		return "", err
	}

	// Preserve all existing Device state and reject another object at this key.
	var device *s4wave_device.Device
	if exists {
		if err := world_types.CheckObjectType(ctx, tx, key, s4wave_device.DeviceTypeID); err != nil {
			return "", err
		}
		device, _, err = world.LookupObject[*s4wave_device.Device](ctx, tx, key, s4wave_device.NewDeviceBlock)
		if err != nil {
			return "", err
		}
		if device.GetPeerId() != authenticatedPeer.String() {
			return "", errors.New("Device record does not match the authenticated peer")
		}
		if device.GetSetupState() == s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY {
			return key, nil
		}
	}

	// Publish session readiness without asserting daemon liveness or capabilities.
	now := timestamppb.New(time.Now())
	if device == nil {
		device = &s4wave_device.Device{PeerId: authenticatedPeer.String(), Label: label, CreatedAt: now.CloneVT()}
	}
	device.SetupState = s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY
	device.UpdatedAt = now
	write := func(cursor *block.Cursor) error {
		cursor.SetBlock(device, true)
		return nil
	}
	if exists {
		_, _, err = world.AccessWorldObject(ctx, tx, key, true, write)
	}
	if !exists {
		_, _, err = world.CreateWorldObject(ctx, tx, key, write)
		if err == nil {
			err = world_types.SetObjectType(ctx, tx, key, s4wave_device.DeviceTypeID)
		}
	}
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return key, nil
}
