//go:build !js && !wasip1

// Package recovery validates surviving native Space replay evidence without opening a provider.
package recovery

import (
	"bytes"
	"context"
	"errors"

	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	world_recovery "github.com/s4wave/spacewave/db/world/recovery"
)

// ValidateWitness returns accepted/before/cover/refused roots only for agreeing native evidence.
// It accepts a bounded record reader and never receives a source store or lifecycle capability.
func ValidateWitness(ctx context.Context, input *world_recovery.WitnessInput, read func([]byte) ([]byte, error)) (*world_recovery.WitnessEvidence, error) {
	// Decode native records selected from the capture, never scanner assertions.
	accepted, refused := &space_world.ReplayCursor{}, &space_world.ReplayCursor{}
	operation, checkpoint := &sobject.SOOperationInner{}, &sobject.SOCheckpointInner{}
	var opData []byte
	for _, item := range []struct {
		key    []byte
		target block.Block
	}{
		{input.GetAcceptedKey(), accepted}, {input.GetRefusedKey(), refused}, {input.GetOperationKey(), operation}, {input.GetCheckpointKey(), checkpoint},
	} {
		data, err := read(item.key)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, errors.New("native causal witness record unavailable")
		}
		if err := item.target.UnmarshalVT(data); err != nil {
			return nil, err
		}
		if item.target == operation {
			opData = data
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Require native structural validity before interpreting causal identities.
	if err := operation.Validate(); err != nil {
		return nil, err
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, err
	}
	hash := sobject.HashSOOperationInner(opData)
	if operation.GetSharedObjectId() != input.GetSpaceId() || checkpoint.GetSharedObjectId() != input.GetSpaceId() || operation.GetPeerId() == "" || operation.GetNonce() == 0 || len(operation.GetOpData()) == 0 {
		return nil, errors.New("causal operation does not bind this Space and a nonempty effect")
	}
	if !bytes.Equal(operation.GetConfigHash(), checkpoint.GetConfigHash()) || checkpoint.GetHeight() <= refused.GetHeight() {
		return nil, errors.New("checkpoint does not follow the refusal under the same configuration")
	}

	// A checkpoint ordinal cannot substitute for exact author-chain membership.
	covered := false
	for _, author := range checkpoint.GetAuthors() {
		if author.GetPeerId() == operation.GetPeerId() && author.GetNonce() == operation.GetNonce() && bytes.Equal(author.GetOpHash(), hash) {
			covered = true
		}
	}
	if !covered {
		return nil, errors.New("checkpoint lacks exact author/nonce/hash cover; later-author chain proof is unavailable")
	}

	// Locate the refused target and require its failed effect to preserve its base.
	refusedFound := false
	var refusal *block.BlockRef
	var refusalBefore *block.BlockRef
	refusedPrevious := refused.GetBase().GetHeadRef().GetRootRef()
	for position, outcome := range refused.GetOutcomes() {
		if bytes.Equal(outcome.GetHash(), hash) && outcome.GetReason() != "" {
			if position != 0 {
				return nil, errors.New("refused prefix transform context is unavailable")
			}
			if !outcome.GetWorld().EqualsRef(refusedPrevious) {
				return nil, errors.New("refused outcome changes its preceding World")
			}
			refusal = outcome.GetWorld()
			refusalBefore = refusedPrevious
			refusedFound = true
		}
		refusedPrevious = outcome.GetWorld()
	}
	if !refusedFound {
		return nil, errors.New("matching refused replay outcome unavailable")
	}

	// Match acceptance to the same exact operation and preceding World.
	previous := accepted.GetBase().GetHeadRef().GetRootRef()
	var after *block.BlockRef
	acceptedFound := false
	for position, outcome := range accepted.GetOutcomes() {
		if bytes.Equal(outcome.GetHash(), hash) && outcome.GetReason() == "" {
			if position != 0 {
				return nil, errors.New("accepted prefix transform context is unavailable")
			}
			if !previous.EqualsRef(refusalBefore) {
				return nil, errors.New("acceptance and refusal have different preceding Worlds")
			}
			after = outcome.GetWorld()
			acceptedFound = true
			break
		}
		previous = outcome.GetWorld()
	}
	if !acceptedFound {
		return nil, errors.New("matching accepted replay outcome unavailable")
	}

	// Require the covering checkpoint to carry that refused replay state.
	state := &space_world.InnerState{}
	if checkpoint.GetKeyEpoch() != 0 {
		return nil, errors.New("encrypted checkpoint state requires an unavailable context")
	}
	if err := state.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return nil, err
	}
	cover := state.GetHeadRef().GetRootRef()
	if !cover.EqualsRef(refusal) {
		return nil, errors.New("checkpoint cover does not carry the refused replay state")
	}

	// Preserve each native interpretation context alongside its semantic role.
	acceptedBefore := accepted.GetBase().GetHeadRef().CloneVT()
	acceptedAfter := acceptedBefore.CloneVT()
	acceptedAfter.RootRef = after

	// Compare the refusal and covering checkpoint under the same full context.
	refusedBefore := refused.GetBase().GetHeadRef().CloneVT()
	refusedAfter := refusedBefore.CloneVT()
	refusedAfter.RootRef = refusal
	if !acceptedBefore.EqualVT(refusedBefore) {
		return nil, errors.New("accepted and refused preceding contexts differ")
	}
	if !state.GetHeadRef().EqualVT(refusedAfter) {
		return nil, errors.New("checkpoint and refused contexts differ")
	}
	return &world_recovery.WitnessEvidence{AcceptedBefore: acceptedBefore, AcceptedAfter: acceptedAfter, RefusedBefore: refusedBefore, RefusedAfter: refusedAfter, Covering: state.GetHeadRef(), OperationHash: hash, Author: operation.GetPeerId(), Nonce: operation.GetNonce()}, nil
}
