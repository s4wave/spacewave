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
	// Validate the Session's peer and storage identities.
	for _, id := range []string{member.GetPeerId(), member.GetStoragePeerId()} {
		if _, _, err := peer.ParsePeerIDWithPubKey(id); err != nil {
			return errors.Wrap(err, "invalid account Session identity")
		}
	}

	// Validate the revocation writer on a revoked Session.
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

	// Keep an already-revoked Session unchanged.
	if current := s.FindAccountSession(member.GetPeerId()); current != nil && current.GetRevoked() {
		return nil
	}

	// Replace the Session's stored copy with the incoming member.
	s.Sessions = slices.DeleteFunc(s.Sessions, func(current *AccountSession) bool {
		return current.GetPeerId() == member.GetPeerId()
	})
	s.Sessions = append(s.Sessions, member.CloneVT())
	return nil
}

// applyCatalogEntry folds one catalog op into the object's record. A tombstone
// of generation g adds g to the record's deleted generations, and becomes the
// record only if the record is absent or is generation g; a live entry of a
// deleted generation is ignored; a live entry of any other generation replaces
// the record. The deleted set survives every replacement. Whether each
// generation is deleted does not depend on the order of the ops, and with at
// most one live generation among them neither does whether the record is live;
// two live generations are last writer wins. A tombstone record's entry is the
// tombstone that replaced it, so which generation it names does depend on the
// order: readers rely on Deleted and the deleted set, never on a tombstone
// record's generation. Entries made before generations carry none; the empty
// generation is a member like any other. A tombstone record written before
// deleted sets counts as deleting its own generation; see DeletesGeneration. A
// tombstone that becomes the record also releases the block store placement,
// which belongs to the id and so to every lifetime of it.
func (s *AccountSettings) applyCatalogEntry(entry *AccountCatalogEntry) error {
	// Validate the entry's resource reference and metadata.
	ref := entry.GetEntry().GetRef()
	if err := ref.Validate(); err != nil {
		return err
	}
	if err := entry.GetEntry().GetMeta().Validate(); err != nil {
		return err
	}

	// Look up the generation in the record's deleted set.
	objectID := ref.GetProviderResourceRef().GetId()
	generation := entry.GetEntry().GetGeneration()
	current := s.FindCatalogEntry(objectID)
	deleted := current.deletedGenerations()
	idx, found := slices.BinarySearch(deleted, generation)

	// A live entry replaces the record unless its generation was deleted.
	if !entry.GetDeleted() {
		if !found {
			s.setCatalogRecord(objectID, &AccountCatalogEntry{Entry: entry.GetEntry().CloneVT(), DeletedGenerations: deleted})
		}
		return nil
	}

	// A tombstone joins the deleted set, and replaces the record only when
	// the record is absent or is its generation.
	if !found {
		deleted = slices.Insert(deleted, idx, generation)
	}
	if current != nil && current.GetEntry().GetGeneration() != generation {
		current.DeletedGenerations = deleted
		return nil
	}
	s.setCatalogRecord(objectID, &AccountCatalogEntry{Entry: entry.GetEntry().CloneVT(), Deleted: true, DeletedGenerations: deleted})

	// A deleted object's blocks no longer need a storage backend.
	s.unplaceBlockStore(ref.GetBlockStoreId())
	return nil
}

// setCatalogRecord replaces the catalog record for objectID with record.
func (s *AccountSettings) setCatalogRecord(objectID string, record *AccountCatalogEntry) {
	s.Catalog = slices.DeleteFunc(s.Catalog, func(current *AccountCatalogEntry) bool {
		return current.GetEntry().GetRef().GetProviderResourceRef().GetId() == objectID
	})
	s.Catalog = append(s.Catalog, record)
}

// DeletesGeneration reports whether a tombstone deleted generation of this
// record's id: the deleted set holds it, or the record is a tombstone of it.
// A tombstone record written before deleted sets existed has an empty set,
// so its own generation counts as deleted too.
func (e *AccountCatalogEntry) DeletesGeneration(generation string) bool {
	_, found := slices.BinarySearch(e.deletedGenerations(), generation)
	return found
}

// deletedGenerations returns a sorted copy of the generations deleted under
// this record's id, the record's own generation included when it is a
// tombstone. See DeletesGeneration.
func (e *AccountCatalogEntry) deletedGenerations() []string {
	// Add a tombstone record's own generation to its set.
	deleted := slices.Clone(e.GetDeletedGenerations())
	if !e.GetDeleted() {
		return deleted
	}
	generation := e.GetEntry().GetGeneration()
	if idx, found := slices.BinarySearch(deleted, generation); !found {
		deleted = slices.Insert(deleted, idx, generation)
	}
	return deleted
}
