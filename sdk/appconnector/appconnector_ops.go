package s4wave_appconnector

import (
	"context"
	"strings"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/sirupsen/logrus"
)

// NewCreateAppConnectorOp constructs a new CreateAppConnectorOp.
func NewCreateAppConnectorOp(
	objKey string,
	label string,
	baseURL string,
	reads []*AppRead,
	tokenSecretObjKey string,
	pollIntervalMs uint32,
	maxBodyBytes uint32,
	ts time.Time,
) *CreateAppConnectorOp {
	return &CreateAppConnectorOp{
		ObjectKey:            objKey,
		Label:                label,
		BaseUrl:              baseURL,
		Reads:                reads,
		TokenSecretObjectKey: tokenSecretObjKey,
		PollIntervalMs:       pollIntervalMs,
		MaxBodyBytes:         maxBodyBytes,
		Timestamp:            timestamppb.New(ts),
	}
}

// Validate checks the op cursorily.
func (o *CreateAppConnectorOp) Validate() error {
	// Require an object key for the connector.
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}

	// Validate the normalized connector and its creation timestamp.
	if err := o.buildAppConnector().Validate(); err != nil {
		return err
	}
	return o.GetTimestamp().Validate(false)
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateAppConnectorOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the operation before changing the World.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Require the token Secret to exist and hold an API token.
	connector := o.buildAppConnector()
	if err := validateTokenSecret(ctx, ws, connector.GetTokenSecretObjectKey()); err != nil {
		return false, err
	}

	// Create the connector object and assign its type.
	createdObject, _, err := world.CreateWorldObject(ctx, ws, o.GetObjectKey(), func(bcs *block.Cursor) error {
		bcs.SetBlock(connector, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}
	if err := world_types.SetObjectType(ctx, ws, o.GetObjectKey(), AppConnectorTypeID); err != nil {
		return false, err
	}

	// Create the empty snapshot object the fetcher writes and assign its type.
	snapshotKey := SnapshotObjectKey(o.GetObjectKey())
	createdObject, _, err = world.CreateWorldObject(ctx, ws, snapshotKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&AppSnapshot{}, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}
	if err := world_types.SetObjectType(ctx, ws, snapshotKey, AppSnapshotTypeID); err != nil {
		return false, err
	}

	// Link the connector to its snapshot.
	if err := ws.SetGraphQuad(ctx, NewAppConnectorToSnapshotQuad(o.GetObjectKey(), snapshotKey)); err != nil {
		return true, err
	}
	return false, nil
}

// buildAppConnector builds the connector block with create-path defaults applied.
func (o *CreateAppConnectorOp) buildAppConnector() *AppConnector {
	// Copy the op fields into a connector stamped at the op time.
	ts := o.GetTimestamp()
	conn := &AppConnector{
		Label:                strings.TrimSpace(o.GetLabel()),
		BaseUrl:              strings.TrimSpace(o.GetBaseUrl()),
		Reads:                o.GetReads(),
		TokenSecretObjectKey: o.GetTokenSecretObjectKey(),
		PollIntervalMs:       o.GetPollIntervalMs(),
		MaxBodyBytes:         o.GetMaxBodyBytes(),
		CreatedAt:            ts,
		UpdatedAt:            ts,
	}

	// Fill the defaults for an unset interval and body limit.
	if conn.PollIntervalMs == 0 {
		conn.PollIntervalMs = DefaultPollIntervalMs
	}
	if conn.MaxBodyBytes == 0 {
		conn.MaxBodyBytes = DefaultMaxBodyBytes
	}
	return conn
}

// validateTokenSecret checks the connector's token reference against redacted Secret metadata.
func validateTokenSecret(ctx context.Context, ws world.WorldState, objKey string) error {
	// Require a Secret object at the referenced key.
	if err := world_types.CheckObjectType(ctx, ws, objKey, s4wave_secret.SecretTypeID); err != nil {
		return errors.Wrap(err, "app connector token secret")
	}

	// Require the Secret to hold an API token.
	secret, err := world.LookupObjectBody[s4wave_secret.Secret](ctx, ws, objKey)
	if err != nil {
		return errors.Wrap(err, "app connector token secret")
	}
	if secret.GetKind() != s4wave_secret.SecretKindAPIToken {
		return world.NewOpRefusedError(errors.Errorf(
			"app connector token secret has kind %q, want %q",
			secret.GetKind(),
			s4wave_secret.SecretKindAPIToken,
		))
	}
	return nil
}

var _ world.Operation = (*CreateAppConnectorOp)(nil)
