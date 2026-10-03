package publish

import (
	"context"
	"os"

	"github.com/pkg/errors"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// EncodeHeadState encodes the plain World state that a CDN checkpoint
// publishes for headRef. Cloud readers resolve the World through their own
// bucket binding, so the bucket id is cleared.
func EncodeHeadState(headRef *bucket.ObjectRef) ([]byte, error) {
	// Encode the head without its bucket binding.
	if headRef == nil || headRef.GetEmpty() {
		return nil, errors.New("space head ref is nil")
	}
	innerHead := headRef.CloneVT()
	innerHead.BucketId = ""
	if err := innerHead.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate head ref")
	}
	return (&sobject_world_engine.InnerState{HeadRef: innerHead}).MarshalVT()
}

// LoadOwnerKey reads the PEM-encoded private key of a CDN Space owner.
func LoadOwnerKey(ownerKeyPem string) (crypto.PrivKey, error) {
	// Parse the PEM key file.
	pemBytes, err := os.ReadFile(ownerKeyPem)
	if err != nil {
		return nil, errors.Wrap(err, "read owner keypair pem")
	}
	defer clear(pemBytes)
	privKey, err := keypem.ParsePrivKeyPem(pemBytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse owner keypair pem")
	}
	return privKey, nil
}

// PostCheckpoint signs and posts the destination checkpoint whose World is
// headRef, after pack upload succeeds. It returns the posted checkpoint body.
func PostCheckpoint(ctx context.Context, opts Options, headRef *bucket.ObjectRef) (*sobject.SOCheckpointInner, error) {
	// CDN pointers can lag creation and recent writes. Only the authenticated
	// destination snapshot is authoritative for the checkpoint to follow.
	data, err := opts.Client.GetSOState(ctx, opts.DstSpaceID, 0, spacewave_provider.SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "fetch destination state")
	}
	msg := &api.SOStateMessage{}
	if err := msg.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode destination state")
	}
	state := msg.GetSnapshot()
	if state == nil {
		return nil, errors.New("destination state did not contain a snapshot")
	}

	// Sign the checkpoint that follows the destination's. A competing
	// publication may still win after the snapshot; the server rejects this
	// checkpoint rather than overwriting the newer one.
	stateData, err := EncodeHeadState(headRef)
	if err != nil {
		return nil, err
	}
	privKey, err := LoadOwnerKey(opts.OwnerKeyPem)
	if err != nil {
		return nil, err
	}
	checkpoint, err := state.BuildNextCheckpoint(opts.DstSpaceID, privKey, stateData)
	if err != nil {
		return nil, errors.Wrap(err, "build destination checkpoint")
	}
	if err := opts.Client.PostCheckpoint(ctx, opts.DstSpaceID, checkpoint); err != nil {
		return nil, errors.Wrap(err, "post destination checkpoint")
	}
	return checkpoint.UnmarshalInner()
}
