package account_settings

import (
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// FindAccountSession returns the approved binding, including a revocation tombstone.
func (s *AccountSettings) FindAccountSession(peerID string) *AccountSession {
	for _, member := range s.GetSessions() {
		if member.GetPeerId() == peerID {
			return member
		}
	}
	return nil
}

// FindCatalogEntry returns the object's catalog record, including its deletion.
func (s *AccountSettings) FindCatalogEntry(objectID string) *AccountCatalogEntry {
	for _, entry := range s.GetCatalog() {
		if entry.GetEntry().GetRef().GetProviderResourceRef().GetId() == objectID {
			return entry
		}
	}
	return nil
}

// applyAccountSession preserves the storage identity approved for a Session.
func (s *AccountSettings) applyAccountSession(member *AccountSession) error {
	for _, id := range []string{member.GetPeerId(), member.GetStoragePeerId()} {
		if _, _, err := peer.ParsePeerIDWithPubKey(id); err != nil {
			return errors.Wrap(err, "invalid account Session identity")
		}
	}
	if writer := member.GetRevokedByStoragePeerId(); writer != "" {
		if !member.GetRevoked() {
			return errors.New("active account Session cannot have a revocation writer")
		}
		if _, _, err := peer.ParsePeerIDWithPubKey(writer); err != nil {
			return errors.Wrap(err, "invalid revocation writer")
		}
	}
	if current := s.FindAccountSession(member.GetPeerId()); current != nil && current.GetStoragePeerId() != member.GetStoragePeerId() {
		return errors.New("account Session storage identity cannot be reassigned")
	}
	if current := s.FindAccountSession(member.GetPeerId()); current != nil && current.GetRevoked() {
		return nil
	}
	s.Sessions = slices.DeleteFunc(s.Sessions, func(current *AccountSession) bool {
		return current.GetPeerId() == member.GetPeerId()
	})
	s.Sessions = append(s.Sessions, member.CloneVT())
	return nil
}

// applyCatalogEntry prevents an offline inventory from resurrecting a deletion.
// A deletion also releases the object's block store placement.
func (s *AccountSettings) applyCatalogEntry(entry *AccountCatalogEntry) error {
	ref := entry.GetEntry().GetRef()
	if err := ref.Validate(); err != nil {
		return err
	}
	if err := entry.GetEntry().GetMeta().Validate(); err != nil {
		return err
	}
	objectID := ref.GetProviderResourceRef().GetId()
	if current := s.FindCatalogEntry(objectID); current != nil && current.GetDeleted() {
		return nil
	}
	s.Catalog = slices.DeleteFunc(s.Catalog, func(current *AccountCatalogEntry) bool {
		return current.GetEntry().GetRef().GetProviderResourceRef().GetId() == objectID
	})
	s.Catalog = append(s.Catalog, entry.CloneVT())

	// A deleted object's blocks no longer need a storage backend.
	if entry.GetDeleted() {
		s.unplaceBlockStore(ref.GetBlockStoreId())
	}
	return nil
}
