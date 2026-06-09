package sobject_world_engine

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/pkg/errors"
)

const spaceWorldRejectedCandidateStoreID = "sobject-world-engine/rejected-candidates"

var spaceWorldRejectedCandidateKeyPrefix = []byte("candidate/")

func (e *soEngine) retainRejectedSpaceWorldCandidate(
	ctx context.Context,
	packet *SpaceWorldFinalizationPacket,
	decision *SpaceWorldFinalizationDecision,
) error {
	if decision.GetStatus() == SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED {
		return nil
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	store, release, err := e.so.AccessLocalStateStore(ctx, spaceWorldRejectedCandidateStoreID, nil)
	if err != nil {
		if e.c != nil && e.c.le != nil {
			e.c.le.WithError(err).Debug("skipping follower-local rejected candidate retention")
		}
		return nil
	}
	if release != nil {
		defer release()
	}
	if store == nil {
		return errors.New("rejected candidate retention store is nil")
	}

	record := &SpaceWorldRejectedCandidate{
		Packet:           packet.CloneVT(),
		Decision:         decision.CloneVT(),
		RetainedUnixNano: uint64(time.Now().UnixNano()), //nolint:gosec
	}
	data, err := record.MarshalVT()
	if err != nil {
		return err
	}

	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, spaceWorldRejectedCandidateKey(packet, decision), data); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (e *soEngine) clearRejectedSpaceWorldCandidate(ctx context.Context, localOperationID string) error {
	store, release, err := e.so.AccessLocalStateStore(ctx, spaceWorldRejectedCandidateStoreID, nil)
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}
	if store == nil {
		return errors.New("rejected candidate retention store is nil")
	}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, spaceWorldRejectedCandidateKeyForID(localOperationID)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func spaceWorldRejectedCandidateKey(
	packet *SpaceWorldFinalizationPacket,
	decision *SpaceWorldFinalizationDecision,
) []byte {
	id := decision.GetLocalOperationId()
	if id == "" {
		id = packet.GetLocalOperationId()
	}
	if id == "" {
		id = hex.EncodeToString(packet.GetCandidateContentId())
	}
	return spaceWorldRejectedCandidateKeyForID(id)
}

func spaceWorldRejectedCandidateKeyForID(id string) []byte {
	key := make([]byte, 0, len(spaceWorldRejectedCandidateKeyPrefix)+len(id))
	key = append(key, spaceWorldRejectedCandidateKeyPrefix...)
	key = append(key, id...)
	return key
}
