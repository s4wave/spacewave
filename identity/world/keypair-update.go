package identity_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// KeypairUpdateOpId is the keypair update operation id.
var KeypairUpdateOpId = KeypairTypeID + "/update"

// NewKeypairUpdateOp constructs a new KeypairUpdateOp block.
func NewKeypairUpdateOp(keypairRef *bucket.ObjectRef) *KeypairUpdateOp {
	return &KeypairUpdateOp{
		KeypairRef: keypairRef,
	}
}

// StoreKeypair stores a keypair to a object using KeypairUpdate.
// If !overwrite, returns if already exists.
// Returns seqno, sysErr, error.
func StoreKeypair(
	ctx context.Context,
	w world.WorldState,
	sender peer.ID,
	kp *identity.Keypair,
	overwrite bool,
) (uint64, bool, error) {
	pid, err := kp.ParsePeerID()
	if err != nil {
		return 0, false, err
	}

	key := NewKeypairKey(pid.String())
	if !overwrite {
		existing, err := LookupKeypairBody(ctx, w, key)
		if err != nil {
			return 0, false, err
		}
		if existing != nil {
			seqno, err := w.GetSeqno(ctx)
			return seqno, false, err
		}
	}

	return storeBlockUpdate(ctx, w, sender, key, kp, func(ref *bucket.ObjectRef) world.Operation {
		return NewKeypairUpdateOp(ref)
	})
}

// LookupOrStoreKeypair looks up the keypair with peer ID or stores a new keypair.
// Returns an error if storing keypair and cannot extract pubkey from peer id.
func LookupOrStoreKeypair(
	ctx context.Context,
	w world.WorldState,
	sender peer.ID,
	keypairPeer peer.ID,
	authMethodID string,
	authMethodParams []byte,
) (*identity.Keypair, string, error) {
	keypairPeerStr := keypairPeer.String()
	if keypairPeer == "" {
		return nil, "", peer.ErrEmptyPeerID
	}
	if err := keypairPeer.Validate(); err != nil {
		return nil, "", err
	}

	kpKey := NewKeypairKey(keypairPeerStr)
	kp, err := LookupKeypairBody(ctx, w, kpKey)
	if err != nil {
		return nil, "", err
	}
	if kp == nil {
		pubKey, err := keypairPeer.ExtractPublicKey()
		if err != nil {
			return nil, "", errors.Wrap(err, "peer_id: extract public key to create keypair")
		}
		kp, err = identity.NewKeypair(pubKey, authMethodID, authMethodParams)
		if err != nil {
			return nil, "", err
		}
		_, _, err = StoreKeypair(ctx, w, sender, kp, false)
		if err != nil {
			return nil, "", err
		}
	}

	return kp, kpKey, nil
}

// EnsureKeypairsExist ensures keypairs are written to storage.
// checks for duplicates in the list
// returns object keys
func EnsureKeypairsExist(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	kps []*identity.Keypair,
	overwrite bool,
) ([]string, error) {
	createdKp := make(map[string]struct{})
	kpObjectKeys := make([]string, len(kps))

	// Resolve object keys and reject duplicate keypair identities.
	for nki, nkp := range kps {
		pid, err := nkp.ParsePeerID()
		if err != nil {
			return nil, errors.Errorf("keypairs[%d]: parse peer id", nki)
		}

		objKey := NewKeypairKey(pid.String())
		kpObjectKeys[nki] = objKey
		if _, dupe := createdKp[objKey]; dupe {
			return nil, errors.Errorf("keypairs[%d]: duplicate: %s", nki, pid.String())
		}
		createdKp[objKey] = struct{}{}
	}

	// Persist each validated keypair.
	for _, kp := range kps {
		_, _, err := StoreKeypair(ctx, ws, sender, kp, overwrite)
		if err != nil {
			return nil, err
		}
	}
	return kpObjectKeys, nil
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *KeypairUpdateOp) Validate() error {
	if err := o.GetKeypairRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *KeypairUpdateOp) GetOperationTypeId() string {
	return KeypairUpdateOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *KeypairUpdateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	kpRef := o.GetKeypairRef()

	resolve := func(ctx context.Context) (string, func() error, error) {
		kp, err := FollowKeypair(ctx, worldHandle.AccessWorldState, kpRef)
		if err != nil {
			return "", nil, err
		}
		validate := func() error {
			if kp.GetPeerId() == "" {
				return errors.New("keypair cannot be empty")
			}
			return kp.Validate()
		}
		pid, err := kp.ParsePeerID()
		if err != nil {
			return "", nil, err
		}
		return NewKeypairKey(pid.String()), validate, nil
	}

	if _, err := applyRefUpdate(ctx, worldHandle, kpRef, KeypairTypeID, resolve); err != nil {
		return false, err
	}
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *KeypairUpdateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Verify the referenced keypair before updating the object root.
	keypairRef := o.GetKeypairRef()
	_, err = FollowKeypair(ctx, objectHandle.AccessWorldState, keypairRef)
	if err != nil {
		return false, err
	}

	// Replace the object's root reference with the validated keypair.
	_, err = objectHandle.SetRootRef(ctx, keypairRef)
	return false, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *KeypairUpdateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *KeypairUpdateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = (*KeypairUpdateOp)(nil)
