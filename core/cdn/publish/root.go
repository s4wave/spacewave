package publish

import (
	"context"
	"os"

	"github.com/pkg/errors"
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
	if headRef == nil || headRef.GetEmpty() {
		return nil, nil, errors.New("space head ref is nil")
	}
	innerHead := headRef.CloneVT()
	innerHead.BucketId = ""
	if err := innerHead.Validate(); err != nil {
		return nil, nil, errors.Wrap(err, "validate head ref")
	}

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

	root := &sobject.SORoot{Inner: innerBytes, InnerSeqno: seqno}
	pemBytes, err := os.ReadFile(validatorKeyPem)
	if err != nil {
		return nil, nil, errors.Wrap(err, "read validator keypair pem")
	}
	validatorPriv, err := keypem.ParsePrivKeyPem(pemBytes)
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
	remoteSeqno, err := FetchRemoteRootSeqno(ctx, opts.CdnBaseURL, opts.DstSpaceID)
	if err != nil {
		return nil, errors.Wrap(err, "fetch destination root pointer")
	}
	root, _, err := BuildSignedRootForHeadRef(opts.DstSpaceID, headRef, opts.ValidatorKeyPem, remoteSeqno+1)
	if err != nil {
		return nil, errors.Wrap(err, "build destination root")
	}
	if err := opts.Client.PostRoot(ctx, opts.DstSpaceID, root, nil); err != nil {
		return nil, errors.Wrap(err, "post destination root")
	}
	return root, nil
}
