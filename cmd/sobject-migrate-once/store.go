package main

import (
	"bytes"
	"cmp"
	"encoding/hex"
	stderrors "errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// Store holds bounded SharedObject metadata from one native volume, never its blocks.
type Store struct {
	// path is the existing native database path.
	path string
	// records is the ordered per-provider object inventory used by the command.
	records []*record
	// objectStores identifies the namespaces visited for the inventory report.
	objectStores []string
}

// record retains one object and its independently prepared repair.
type record struct {
	// id is the SharedObject identity.
	id string
	// provider identifies local or externally authoritative state.
	provider string
	// account identifies the provider account.
	account string
	// name is the optional Space metadata name.
	name string
	// base is the raw object-store prefix, including its trailing slash.
	base string
	// state is the decoded current host or cache state, when stored locally.
	state *sobject.SOState
	// history is the retained configuration chain, oldest first.
	history []*sobject.SOConfigChange
	// next is a validated proposed repair, nil when no write is authorized.
	next *sobject.SOState
	// genesis is the sole proposed configuration history entry.
	genesis *sobject.SOConfigChange
}

// ReadStore reads identities, config history, and available keys without sidecar writes.
func ReadStore(path string, keys keySet) (store *Store, err error) {
	// Open an exclusive read-only snapshot without touching coordination sidecars.
	db, err := openDatabase(path, true)
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, db.Close()) }()
	store = &Store{path: path}

	// Visit only ObjectStore records and the volume identity, excluding block data.
	err = db.View(func(tx *bdb.Tx) error {
		// Decode the persistent storage identity without exposing its bytes.
		bucket := tx.Bucket([]byte("hydra"))
		if bucket == nil {
			return nil
		}
		encoding := kvkey.NewDefaultKVKey()
		var storageKey [32]byte
		defer scrub.Scrub(storageKey[:])
		if data := bucket.Get(encoding.GetPeerPrivKey()); len(data) != 0 {
			key, err := keypem.ParsePrivKeyPem(data)
			if err != nil {
				return errors.New("volume private key could not be decoded")
			}
			if err := keys.add(key); err != nil {
				return err
			}
			storageKey, err = session_lock.DeriveStorageKey(key)
			if err != nil {
				return errors.New("volume storage key could not be derived")
			}
		}

		// Catalog host states and Space names by provider, account, and object ID.
		byID := make(map[string]*record)
		namespaces := make(map[string]struct{})
		prefix := encoding.GetObjectStorePrefixByID("")
		prefix = prefix[:len(prefix)-1]
		cursor := bucket.Cursor()
		for key, value := cursor.Seek(prefix); bytes.HasPrefix(key, prefix); key, value = cursor.Next() {
			// Split only provider-owned ObjectStore grammars understood by this tool.
			parts := strings.Split(string(key[len(prefix):]), "/")
			if len(parts) >= 4 && parts[0] == "cdn" && parts[2] == "pack-index" {
				// A durable CDN pack index identifies a Space without persisting its root pointer.
				namespace := strings.Join(parts[:3], "/")
				namespaces[namespace] = struct{}{}
				index := "cdn/" + parts[1]
				if byID[index] == nil {
					byID[index] = &record{id: parts[1], provider: "cdn"}
				}
				continue
			}
			if len(parts) < 5 || parts[0] != "p" {
				continue
			}
			provider, account := parts[1], parts[2]
			base := string(prefix) + strings.Join(parts[:4], "/") + "/"
			namespaces[strings.Join(parts[:4], "/")] = struct{}{}
			identity := func(id, provider, account string) *record {
				// Merge catalog metadata and host state under one provider identity.
				index := provider + "/" + account + "/" + id
				r := byID[index]
				if r == nil {
					r = &record{id: id, provider: provider, account: account, base: base}
					byID[index] = r
				}
				return r
			}

			// Read local host states and the provider's plaintext Space catalog.
			switch {
			case len(parts) == 7 && parts[3] == "so" && parts[4] == "so" && parts[6] == "host":
				r := identity(parts[5], provider, account)
				r.base = base
				r.state = &sobject.SOState{}
				if err := r.state.UnmarshalVT(value); err != nil {
					return errors.Wrapf(err, "decode host %s", r.id)
				}
			case len(parts) == 5 && parts[4] == "so-list":
				list := &sobject.SharedObjectList{}
				if err := list.UnmarshalVT(value); err != nil {
					return errors.Wrap(err, "decode object catalog")
				}
				for _, entry := range list.GetSharedObjects() {
					// Read Space names from generated metadata without decrypting a World.
					ref := entry.GetRef().GetProviderResourceRef()
					r := identity(ref.GetId(), ref.GetProviderId(), ref.GetProviderAccountId())
					if entry.GetMeta().GetBodyType() == space.SpaceBodyType {
						meta := &space.SpaceSoMeta{}
						if err := meta.UnmarshalVT(entry.GetMeta().GetBodyMeta()); err != nil {
							return errors.Wrap(err, "decode Space metadata")
						}
						r.name = meta.GetName()
					}
				}
			case len(parts) == 6 && parts[3] == "sess" && parts[5] == "pk":
				// Auto-unlock session keys stay in memory; PIN-protected keys remain unavailable.
				plain, err := session_lock.DecryptAutoUnlock(storageKey, value)
				if err != nil {
					return errors.New("auto-unlock session key could not be decrypted")
				}
				key, err := keypem.ParsePrivKeyPem(plain)
				scrub.Scrub(plain)
				if err != nil {
					return errors.New("auto-unlock session key could not be decoded")
				}
				if err := keys.add(key); err != nil {
					return err
				}
			case len(parts) == 6 && parts[4] == "verified-so-state":
				// Inspect cloud cache evidence without scheduling any repair.
				cache := &api.VerifiedSOStateCache{}
				if err := cache.UnmarshalVT(value); err != nil {
					return errors.Wrap(err, "decode verified cloud cache")
				}
				r := identity(parts[5], provider, account)
				r.state = cache.GetPeerState()
				if r.state == nil {
					r.state = cache.GetCloudState()
				}
				r.history = cache.GetConfigHistory()
			}
		}

		// Read each local object's retained chain through its narrow key prefix.
		for _, r := range byID {
			// Skip externally authoritative records and absent host state.
			if r.provider == "local" && r.state != nil {
				prefix := []byte(r.base + "so/" + r.id + "/config-history/entry/")
				for key, data := cursor.Seek(prefix); bytes.HasPrefix(key, prefix); key, data = cursor.Next() {
					// Decode immutable entries in the pinned volume snapshot.
					entry := &sobject.SOConfigChange{}
					if err := entry.UnmarshalVT(data); err != nil {
						return errors.Wrapf(err, "decode history %s", r.id)
					}
					r.history = append(r.history, entry)
				}
				slices.SortFunc(r.history, func(a, b *sobject.SOConfigChange) int {
					return cmp.Compare(a.GetConfigSeqno(), b.GetConfigSeqno())
				})
			}
			store.records = append(store.records, r)
		}
		slices.SortFunc(store.records, func(a, b *record) int {
			return strings.Compare(a.provider+"/"+a.account+"/"+a.id, b.provider+"/"+b.account+"/"+b.id)
		})
		for namespace := range namespaces {
			store.objectStores = append(store.objectStores, namespace)
		}
		slices.Sort(store.objectStores)
		return nil
	})
	return store, err
}

// signers selects local keys with the exact owner and validator authorities required.
func (r *record) signers(keys keySet) (owner, validator crypto.PrivKey) {
	// Match public participant identities, never infer authority from key presence.
	for _, participant := range r.state.GetConfig().GetParticipants() {
		key := keys[participant.GetPeerId()]
		if key == nil {
			continue
		}
		if owner == nil && sobject.IsOwner(participant.GetRole()) {
			owner = key
		}
		if validator == nil && sobject.IsValidatorOrOwner(participant.GetRole()) {
			validator = key
		}
	}
	return owner, validator
}

// validation reports state, root consensus, and full retained history independently.
func (r *record) validation() (state, chain string) {
	// Missing local snapshots cannot attest remotely authoritative state.
	if r.state == nil {
		return "no stored host snapshot", "no stored full chain"
	}

	// Run master's validators against the current decoded state and history.
	state, chain = "valid", "valid"
	if err := r.state.Validate(r.id); err != nil {
		state = err.Error()
	}
	if state == "valid" {
		if err := validateRoot(r.id, r.state); err != nil {
			state = err.Error()
		}
	}
	if err := sobject.VerifyConfigChain(r.id, r.history); err != nil {
		chain = err.Error()
	}
	if chain == "valid" {
		// Bind the verified history head to the configuration stored with this root.
		last := r.history[len(r.history)-1]
		head, err := sobject.HashSOConfigChange(last)
		if err != nil {
			return state, err.Error()
		}
		config := last.GetConfig().CloneVT()
		config.ConfigChainHash = head
		config.ConfigChainSeqno = last.GetConfigSeqno()
		if !sobject.EqualSOConfigs(config, r.state.GetConfig()) {
			chain = "verified history does not match current configuration"
		}
	}
	return state, chain
}

// Write commits each proposed repair independently, then reopens and compares it.
func (s *Store) Write() (err error) {
	// Leave a store with no proposed local repair completely unopened for writing.
	if !slices.ContainsFunc(s.records, func(r *record) bool { return r.next != nil }) {
		return nil
	}
	db, err := openDatabase(s.path, false)
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, db.Close()) }()

	// Replace old history, host state, and local queued envelopes in one SO transaction.
	for _, r := range s.records {
		if r.next == nil {
			continue
		}
		if err := db.Update(func(tx *bdb.Tx) error { return r.write(tx) }); err != nil {
			return err
		}
	}
	if err := db.Close(); err != nil {
		return err
	}

	// Reopen the physical volume and compare every written record with the prepared repair.
	reopened, err := ReadStore(s.path, make(keySet))
	if err != nil {
		return err
	}
	for _, expected := range s.records {
		// Require exactly the prepared state and one valid genesis after reopening.
		if expected.next == nil {
			continue
		}
		index := slices.IndexFunc(reopened.records, func(r *record) bool {
			return r.provider == expected.provider && r.account == expected.account && r.id == expected.id
		})
		if index == -1 {
			return errors.Errorf("reopened object missing: %s", expected.id)
		}
		actual := reopened.records[index]
		state, chain := actual.validation()
		if state != "valid" || chain != "valid" || !actual.state.EqualVT(expected.next) || len(actual.history) != 1 || !actual.history[0].EqualVT(expected.genesis) {
			return errors.Errorf("reopened object differs or fails validation: %s", expected.id)
		}
	}
	return s.verifyAuxiliary()
}

// write atomically replaces one object's history and state after detecting stale input.
func (r *record) write(tx *bdb.Tx) error {
	// Require the same stored host snapshot used to prepare this repair.
	bucket := tx.Bucket([]byte("hydra"))
	key := []byte(r.base + "so/" + r.id + "/host")
	current := &sobject.SOState{}
	if err := current.UnmarshalVT(bucket.Get(key)); err != nil {
		return err
	}
	if !current.EqualVT(r.state) {
		return errors.Errorf("host changed after inventory: %s", r.id)
	}

	// Remove the old retained history before writing a single genesis checkpoint.
	prefix := []byte(r.base + "so/" + r.id + "/config-history/")
	cursor := bucket.Cursor()
	var oldKeys [][]byte
	for key, _ := cursor.Seek(prefix); bytes.HasPrefix(key, prefix); key, _ = cursor.Next() {
		oldKeys = append(oldKeys, bytes.Clone(key))
	}
	for _, key := range oldKeys {
		if err := bucket.Delete(key); err != nil {
			return err
		}
	}
	entryHash, err := sobject.HashSOConfigChange(r.genesis)
	if err != nil {
		return err
	}

	// Serialize the host, resulting checkpoint, genesis, and empty local queue together.
	values := []struct {
		// key is the raw ObjectStore record key.
		key string
		// value is a generated protobuf encoder for this record.
		value interface{ MarshalVT() ([]byte, error) }
	}{
		{string(key), r.next},
		{string(prefix) + "checkpoint", r.next.GetConfig()},
		{string(prefix) + "entry/" + hex.EncodeToString(entryHash), r.genesis},
	}
	for _, value := range values {
		// Encode each protobuf with its own generated codec before adding it to the write.
		data, err := value.value.MarshalVT()
		if err != nil {
			return err
		}
		if err := bucket.Put([]byte(value.key), data); err != nil {
			return err
		}
	}
	return bucket.Delete([]byte(r.base + "so/" + r.id + "/local/state"))
}

// verifyAuxiliary checks the persisted checkpoint and cleared local queue after reopening.
func (s *Store) verifyAuxiliary() (err error) {
	// Read the written volume through a new sidecar-free snapshot.
	db, err := openDatabase(s.path, true)
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, db.Close()) }()
	return db.View(func(tx *bdb.Tx) error {
		// Verify each repaired object's checkpoint and local queue in the pinned snapshot.
		bucket := tx.Bucket([]byte("hydra"))
		for _, r := range s.records {
			if r.next == nil {
				continue
			}
			prefix := r.base + "so/" + r.id + "/"
			checkpoint := &sobject.SharedObjectConfig{}
			if err := checkpoint.UnmarshalVT(bucket.Get([]byte(prefix + "config-history/checkpoint"))); err != nil {
				return err
			}
			if !checkpoint.EqualVT(r.next.GetConfig()) || bucket.Get([]byte(prefix+"local/state")) != nil {
				return errors.Errorf("checkpoint or local queue differs: %s", r.id)
			}
		}
		return nil
	})
}

// openDatabase fails immediately on any live database holder instead of waiting.
// Read-only handles use an fd pathname with no sidecar, since bbolt's ordinary
// read-only open still modifies reader slots in the persistent coordination files.
func openDatabase(path string, readOnly bool) (*bdb.DB, error) {
	return bdb.Open(path, 0o600, &bdb.Options{
		ReadOnly: readOnly, Exclusive: true, FreelistType: bdb.FreelistMapType,
		OpenFile: func(path string, flags int, mode os.FileMode) (*os.File, error) {
			// Open an existing database and acquire its maintenance lock without a retry.
			flags &^= os.O_CREATE
			file, err := os.OpenFile(path, flags, mode)
			if err != nil {
				return nil, err
			}
			if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
				return nil, stderrors.Join(errors.Wrap(err, "live database lock; refuse maintenance"), file.Close())
			}
			if !readOnly {
				return file, nil
			}

			// Preserve the locked open-file description under a sidecar-free fd name.
			fd, err := syscall.Dup(int(file.Fd()))
			if err != nil {
				return nil, stderrors.Join(err, file.Close())
			}
			if err := file.Close(); err != nil {
				return nil, stderrors.Join(err, syscall.Close(fd))
			}
			return os.NewFile(uintptr(fd), "/dev/fd/"+strconv.Itoa(fd)), nil
		},
	})
}
