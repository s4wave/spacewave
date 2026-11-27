package s4wave_secret

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// CreateSecretOptions configures CreateSecret.
type CreateSecretOptions struct {
	// ObjectKey is the parent World object key.
	ObjectKey string
	// SpaceID is the Space whose World holds ObjectKey. It is written into
	// the payload as its owner, so only a Secret delete in this Space can
	// remove the payload.
	SpaceID string
	// DisplayName is the parent Secret display name.
	DisplayName string
	// Kind is the Secret kind.
	Kind string
	// ContentType is the payload content type.
	ContentType string
	// Value is the raw payload stored in the nested SharedObject.
	Value []byte
	// Timestamp is used for created_at and updated_at.
	Timestamp time.Time
	// NestedSharedObjectId optionally fixes the nested SharedObject id.
	NestedSharedObjectId string
}

// CreateSecret creates a parent World object plus nested SharedObject payload.
// It returns world.ErrObjectExists when opts.ObjectKey is taken, so a caller
// that keys its Secrets can create one again by key and, on that error,
// replace the payload of the one that exists instead. A caller that lost the
// race for the key leaves no nested SharedObject behind.
func CreateSecret(
	ctx context.Context,
	b bus.Bus,
	soProvider sobject.SharedObjectProvider,
	engine world.Engine,
	opts CreateSecretOptions,
) (*Secret, error) {
	// Require a free parent World object key before creating the nested payload.
	if opts.ObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}
	if err := checkSecretKeyFree(ctx, engine, opts.ObjectKey); err != nil {
		return nil, err
	}

	// Create the nested SharedObject and its payload.
	secret, err := CreateSecretObject(ctx, b, soProvider, opts)
	if err != nil {
		return nil, err
	}
	if err := commitSecretObject(ctx, engine, opts.ObjectKey, secret); err != nil {
		// Another caller took the key between the check and this
		// commit, so no World object references this nested
		// SharedObject: remove it.
		if errors.Is(err, world.ErrObjectExists) {
			if derr := soProvider.DeleteSharedObject(ctx, secret.GetNestedSharedObjectId()); derr != nil {
				return nil, errors.Wrap(derr, "remove unused nested shared object")
			}
		}
		return nil, err
	}
	return secret, nil
}

// checkSecretKeyFree returns world.ErrObjectExists when objectKey references an
// object in the World.
func checkSecretKeyFree(ctx context.Context, engine world.Engine, objectKey string) error {
	// Read the key in a read-only transaction.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	obj, found, err := tx.GetObject(ctx, objectKey)
	world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if found {
		return errors.Wrap(world.ErrObjectExists, objectKey)
	}
	return nil
}

// commitSecretObject writes the redacted Secret metadata as the parent World
// object at objectKey, typed as a Secret, in one transaction.
func commitSecretObject(ctx context.Context, engine world.Engine, objectKey string, secret *Secret) error {
	// Create the object and set its type in one write transaction.
	wtx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	createdObject, _, err := world.CreateWorldObject(ctx, wtx, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(secret, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		wtx.Discard()
		return err
	}
	if err := world_types.SetObjectType(ctx, wtx, objectKey, SecretTypeID); err != nil {
		wtx.Discard()
		return err
	}
	return wtx.Commit(ctx)
}

// CreateSecretObject creates the nested SharedObject holding a Secret payload
// and returns the redacted Secret metadata. It writes no World object, so the
// caller records the metadata wherever the Secret is used. The payload records
// opts.SpaceID as its owner. Without opts.SpaceID, the payload records no
// owner and a Secret delete keeps the payload.
func CreateSecretObject(
	ctx context.Context,
	b bus.Bus,
	soProvider sobject.SharedObjectProvider,
	opts CreateSecretOptions,
) (*Secret, error) {
	// Fill the defaults for the timestamp, content type, and nested id.
	if opts.Timestamp.IsZero() {
		opts.Timestamp = time.Now()
	}
	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}
	nestedID := opts.NestedSharedObjectId
	if nestedID == "" {
		nestedID = "secret-" + sobject.NewSOOperationLocalID()
	}

	// Create the nested SharedObject and store the first payload version.
	nestedRef, err := soProvider.CreateSharedObject(ctx, nestedID, NewSharedObjectMeta(), "", "")
	if err != nil {
		return nil, errors.Wrap(err, "create nested shared object")
	}
	payload := &SecretPayload{
		Value:       bytes.Clone(opts.Value),
		ContentType: opts.ContentType,
		Version:     1,
		UpdatedAt:   timestamppb.New(opts.Timestamp),
	}
	payload.OwnerSpaceId = opts.SpaceID
	if err := StoreSecretPayload(ctx, b, nestedRef, payload); err != nil {
		return nil, errors.Wrap(err, "store secret payload")
	}

	return &Secret{
		DisplayName:          opts.DisplayName,
		Kind:                 opts.Kind,
		NestedSharedObjectId: nestedRef.GetProviderResourceRef().GetId(),
		Ref:                  nestedRef.CloneVT(),
		CreatedAt:            timestamppb.New(opts.Timestamp),
		UpdatedAt:            timestamppb.New(opts.Timestamp),
	}, nil
}
