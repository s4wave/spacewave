package sobject

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"slices"

	"github.com/pkg/errors"
)

func journalIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

const journalScopeIDSize = sha256.Size

// NewSOMutationKey constructs the immutable identity for one exact attempt.
func NewSOMutationKey(originScopeID []byte, sharedObjectID, participantPeerID, localID string) (*SOMutationKey, error) {
	key := &SOMutationKey{
		OriginScopeId:     slices.Clone(originScopeID),
		SharedObjectId:    sharedObjectID,
		ParticipantPeerId: participantPeerID,
		LocalId:           localID,
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	return key, nil
}

// Validate checks the complete participant-scoped mutation identity.
func (k *SOMutationKey) Validate() error {
	if k == nil || len(k.GetOriginScopeId()) != journalScopeIDSize ||
		k.GetSharedObjectId() == "" || k.GetParticipantPeerId() == "" || k.GetLocalId() == "" {
		return ErrJournalInvalidKey
	}
	return nil
}

// EqualExact compares every identity component, including the origin scope.
func (k *SOMutationKey) EqualExact(other *SOMutationKey) bool {
	if k == nil || other == nil {
		return k == other
	}
	return bytes.Equal(k.GetOriginScopeId(), other.GetOriginScopeId()) &&
		k.GetSharedObjectId() == other.GetSharedObjectId() &&
		k.GetParticipantPeerId() == other.GetParticipantPeerId() &&
		k.GetLocalId() == other.GetLocalId()
}

// MutationKeyDigest returns the stable digest used to identify an exact key in indexes.
// Only the four semantic identity fields participate; protobuf unknown fields do not.
func MutationKeyDigest(k *SOMutationKey) ([]byte, error) {
	if err := k.Validate(); err != nil {
		return nil, err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("spacewave/sharedobject/mutation-key/v1"))
	writeField := func(data []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(data)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(data)
	}
	writeField(k.GetOriginScopeId())
	writeField([]byte(k.GetSharedObjectId()))
	writeField([]byte(k.GetParticipantPeerId()))
	writeField([]byte(k.GetLocalId()))
	return digest.Sum(nil), nil
}

func validateJournalLineage(key *SOMutationKey, lineage *SOJournalLineage) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if lineage == nil || lineage.GetRootKey() == nil || !lineage.GetRootKey().EqualExact(key) {
		return errors.Wrap(ErrJournalInvalidKey, "lineage root key does not match record key")
	}
	if supersedes := lineage.GetSupersedes(); supersedes != nil {
		if err := supersedes.Validate(); err != nil {
			return errors.Wrap(err, "lineage supersedes key")
		}
		if supersedes.EqualExact(key) {
			return ErrJournalSupersessionImmutable
		}
		if !bytes.Equal(supersedes.GetOriginScopeId(), key.GetOriginScopeId()) ||
			supersedes.GetSharedObjectId() != key.GetSharedObjectId() ||
			supersedes.GetParticipantPeerId() != key.GetParticipantPeerId() {
			return ErrJournalSupersessionImmutable
		}
	}
	return nil
}

func cloneJournalLineage(lineage *SOJournalLineage) *SOJournalLineage {
	if lineage == nil {
		return nil
	}
	return lineage.CloneVT()
}
