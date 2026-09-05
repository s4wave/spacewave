package publish

import (
	"context"
	"math"
	"os"

	"github.com/pkg/errors"
	spacewave_provider "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/keypem"
)

// BuildSignedRootForHeadRef signs a new SORoot for a Space/head ref at seqno.
func BuildSignedRootForHeadRef(
	spaceID string,
	headRef *bucket.ObjectRef,
	validatorKeyPem string,
	seqno uint64,
) (*sobject.SORoot, []byte, error) {
	// Cloud readers resolve the World through their own bucket binding.
	if headRef == nil || headRef.GetEmpty() {
		return nil, nil, errors.New("space head ref is nil")
	}
	innerHead := headRef.CloneVT()
	innerHead.BucketId = ""
	if err := innerHead.Validate(); err != nil {
		return nil, nil, errors.Wrap(err, "validate head ref")
	}

	// The signed envelope binds this World head to the destination sequence.
	innerState := &sobject_world_engine.InnerState{HeadRef: innerHead}
	stateData, err := innerState.MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal inner state")
	}

	sori := &sobject.SORootInner{Seqno: seqno, StateData: stateData}
	innerBytes, err := sori.MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal SORootInner")
	}

	// Read existing signing material without generating a replacement identity.
	root := &sobject.SORoot{Inner: innerBytes, InnerSeqno: seqno}
	pemBytes, err := os.ReadFile(validatorKeyPem)
	if err != nil {
		return nil, nil, errors.Wrap(err, "read validator keypair pem")
	}
	validatorPriv, err := keypem.ParsePrivKeyPem(pemBytes)
	clear(pemBytes)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse validator keypair pem")
	}
	if err := root.SignInnerData(validatorPriv, spaceID, seqno, hash.RecommendedHashType); err != nil {
		return nil, nil, errors.Wrap(err, "sign SORoot inner data")
	}
	rootBytes, err := root.MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal signed SORoot")
	}
	return root, rootBytes, nil
}

// PostRoot signs and posts the destination root after pack upload succeeds.
func PostRoot(ctx context.Context, opts Options, headRef *bucket.ObjectRef) (*sobject.SORoot, error) {
	// CDN pointers can lag creation and recent writes. Only the authenticated
	// destination snapshot is authoritative for the next publication sequence.
	data, err := opts.Client.GetSOState(ctx, opts.DstSpaceID, 0, spacewave_provider.SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "fetch destination state")
	}
	state := &api.SOStateMessage{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode destination state")
	}
	if state.GetSnapshot() == nil {
		return nil, errors.New("destination state did not contain a snapshot")
	}
	remoteSeqno := state.GetSnapshot().GetRoot().GetInnerSeqno()
	if remoteSeqno == math.MaxUint64 {
		return nil, errors.New("destination root sequence is exhausted")
	}

	// A competing publication may still win after the snapshot; retain the
	// server's conflict response rather than overwriting its newer root.
	root, _, err := BuildSignedRootForHeadRef(opts.DstSpaceID, headRef, opts.ValidatorKeyPem, remoteSeqno+1)
	if err != nil {
		return nil, errors.Wrap(err, "build destination root")
	}
	if err := opts.Client.PostRoot(ctx, opts.DstSpaceID, root, nil); err != nil {
		return nil, errors.Wrap(err, "post destination root")
	}
	return root, nil
}
