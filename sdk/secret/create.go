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
func CreateSecret(
	ctx context.Context,
	b bus.Bus,
	soProvider sobject.SharedObjectProvider,
	engine world.Engine,
	opts CreateSecretOptions,
) (*Secret, error) {
	// Require the parent World object key before creating the nested payload.
	if opts.ObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}

	// Create the nested SharedObject and its payload.
	secret, err := CreateSecretObject(ctx, b, soProvider, opts)
	if err != nil {
		return nil, err
	}

	// Write the redacted Secret metadata as the parent World object.
	wtx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	createdObject, _, err := world.CreateWorldObject(ctx, wtx, opts.ObjectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(secret, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		wtx.Discard()
		return nil, err
	}
	if err := world_types.SetObjectType(ctx, wtx, opts.ObjectKey, SecretTypeID); err != nil {
		wtx.Discard()
		return nil, err
	}
	if err := wtx.Commit(ctx); err != nil {
		return nil, err
	}
	return secret, nil
}

// CreateSecretObject creates the nested SharedObject holding a Secret payload
// and returns the redacted Secret metadata. It writes no World object, so the
// caller records the metadata wherever the Secret is used. The payload names
// opts.SpaceID as its owner; without one it names no owner and a Secret delete
// keeps it.
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
