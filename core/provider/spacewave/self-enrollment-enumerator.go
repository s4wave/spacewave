package provider_spacewave

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// SelfEnrollmentSummary is the cached post-login self-enrollment predicate.
type SelfEnrollmentSummary struct {
	ids           []string
	generationKey string
	count         uint32
	loaded        bool
}

func (a *ProviderAccount) RefreshSelfEnrollmentSummary(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.soListCtr == nil {
		return a.setSelfEnrollmentSummary(nil)
	}
	list := a.soListCtr.GetValue()
	if list == nil {
		if !a.hasSharedObjectListAccess() {
			return a.setSelfEnrollmentSummary(&SelfEnrollmentSummary{loaded: true})
		}
		if err := a.EnsureSharedObjectListLoaded(ctx); err != nil {
			return err
		}
		list = a.soListCtr.GetValue()
		if list == nil {
			return a.setSelfEnrollmentSummary(nil)
		}
	}
	sessionPeerID := a.GetCurrentSessionPeerID()
	if sessionPeerID == "" {
		return a.setSelfEnrollmentSummary(nil)
	}
	entityID, err := a.GetSelfEntityID(ctx)
	if err != nil {
		return err
	}
	summary, err := a.enumerateSelfEnrollmentCandidates(
		ctx,
		list,
		sessionPeerID,
		entityID,
	)
	if err != nil {
		return err
	}
	return a.setSelfEnrollmentSummary(summary)
}

func (a *ProviderAccount) GetSelfEnrollmentSummary() *SelfEnrollmentSummary {
	summary := a.state.selfEnrollmentSummary
	if summary == nil {
		return nil
	}
	return summary.clone()
}

func (a *ProviderAccount) GetSelfEnrollmentSkippedGenerationKey() string {
	return a.state.selfEnrollmentSkippedGenerationKey
}

func (a *ProviderAccount) SetSelfEnrollmentSkippedGenerationKey(key string) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.state.selfEnrollmentSkippedGenerationKey == key {
			return
		}
		a.state.selfEnrollmentSkippedGenerationKey = key
		broadcast()
	})
}

// GetIDs returns the shared object IDs needing self-enrollment.
func (s *SelfEnrollmentSummary) GetIDs() []string {
	return slices.Clone(s.ids)
}

// GetGenerationKey returns the generation key for the current pending set.
func (s *SelfEnrollmentSummary) GetGenerationKey() string {
	return s.generationKey
}

// GetCount returns the number of shared objects needing self-enrollment.
func (s *SelfEnrollmentSummary) GetCount() uint32 {
	return s.count
}

// GetLoaded returns whether every shared-object list entry was evaluated.
func (s *SelfEnrollmentSummary) GetLoaded() bool {
	return s.loaded
}

func (a *ProviderAccount) enumerateSelfEnrollmentCandidates(
	ctx context.Context,
	list *sobject.SharedObjectList,
	sessionPeerID peer.ID,
	entityID string,
) (*SelfEnrollmentSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded := true
	var ids []string
	for _, entry := range list.GetSharedObjects() {
		ref := entry.GetRef()
		soID := ref.GetProviderResourceRef().GetId()
		excluded, err := a.isSelfEnrollmentExcludedSharedObject(ctx, entry)
		if err != nil {
			return nil, err
		}
		if soID == "" || excluded {
			continue
		}
		cache, err := a.loadVerifiedSOStateCache(ctx, soID)
		if err != nil {
			return nil, err
		}
		if cache == nil || cache.GetCurrentConfig() == nil {
			ids = append(ids, soID)
			continue
		}
		role := readableParticipantRoleForEntity(cache.GetCurrentConfig(), entityID)
		if !sobject.CanReadState(role) {
			continue
		}
		if peerEnrolledInCurrentEpoch(cache.GetKeyEpochs(), sessionPeerID.String()) {
			continue
		}
		ids = append(ids, soID)
	}
	slices.Sort(ids)
	var key string
	if loaded {
		key = buildSelfEnrollmentGenerationKey(ids, sessionPeerID)
	}
	return &SelfEnrollmentSummary{
		ids:           ids,
		generationKey: key,
		count:         uint32(len(ids)),
		loaded:        loaded,
	}, nil
}

func (a *ProviderAccount) refreshSelfEnrollmentSummary(ctx context.Context) {
	if err := a.RefreshSelfEnrollmentSummary(ctx); err != nil {
		if a.le != nil {
			a.le.WithError(err).Debug("failed to refresh self-enrollment summary")
		}
	}
}

func (a *ProviderAccount) setSelfEnrollmentSummary(summary *SelfEnrollmentSummary) error {
	next := summary.clone()
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if equalSelfEnrollmentSummary(a.state.selfEnrollmentSummary, next) {
			return
		}
		a.state.selfEnrollmentSummary = next
		broadcast()
	})
	return nil
}

func (s *SelfEnrollmentSummary) clone() *SelfEnrollmentSummary {
	if s == nil {
		return nil
	}
	return &SelfEnrollmentSummary{
		ids:           slices.Clone(s.ids),
		generationKey: s.generationKey,
		count:         s.count,
		loaded:        s.loaded,
	}
}

func equalSelfEnrollmentSummary(a, b *SelfEnrollmentSummary) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.generationKey == b.generationKey &&
		a.count == b.count &&
		a.loaded == b.loaded &&
		slices.Equal(a.ids, b.ids)
}

func (a *ProviderAccount) isSelfEnrollmentExcludedSharedObject(
	ctx context.Context,
	entry *sobject.SharedObjectListEntry,
) (bool, error) {
	if entry.GetMeta().GetBodyType() == "cdn" {
		return true, nil
	}
	ref := entry.GetRef()
	if ref == nil || ref.GetProviderResourceRef() == nil {
		return true, nil
	}
	soID := ref.GetProviderResourceRef().GetId()
	if soID == "" {
		return true, nil
	}
	metadata, err := a.GetSharedObjectMetadata(ctx, soID)
	if err == ErrSharedObjectMetadataDeleted {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return metadata.GetPublicRead(), nil
}

func buildSelfEnrollmentGenerationKey(ids []string, sessionPeerID peer.ID) string {
	if len(ids) == 0 {
		return ""
	}
	h := sha256.New()
	for _, id := range ids {
		_, _ = h.Write([]byte(id))
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write([]byte(sessionPeerID))
	return hex.EncodeToString(h.Sum(nil))
}
