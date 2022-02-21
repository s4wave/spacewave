package identity_world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/identity"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
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

	pidPretty := pid.Pretty()
	key := NewKeypairKey(pidPretty)
	seqno, err := w.GetSeqno()
	if err != nil {
		return 0, false, err
	}
	obj, objFound, err := w.GetObject(key)
	if err != nil {
		return 0, false, err
	}
	if objFound && !overwrite {
		return seqno, false, nil
	}
	setKeypair := func(bcs *block.Cursor) error {
		bcs.SetBlock(kp, true)
		bcs.ClearAllRefs()
		return nil
	}
	var kpRef *bucket.ObjectRef
	if objFound {
		var changed bool
		kpRef, changed, err = world.AccessObjectState(ctx, obj, false, setKeypair)
		if err != nil || !changed {
			return 0, false, err
		}
	} else {
		kpRef, err = world.AccessObject(ctx, w.AccessWorldState, nil, setKeypair)
		if err != nil {
			return 0, false, err
		}
	}

	op := NewKeypairUpdateOp(kpRef)
	return w.ApplyWorldOp(op, sender)
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
	keypairPeerStr := keypairPeer.Pretty()
	if keypairPeer == "" {
		return nil, "", peer.ErrPeerIDEmpty
	}
	if err := keypairPeer.Validate(); err != nil {
		return nil, "", err
	}

	kpKey := NewKeypairKey(keypairPeerStr)
	kp, _, err := LookupKeypair(ctx, w, kpKey)
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
	for nki, nkp := range kps {
		pid, err := nkp.ParsePeerID()
		if err != nil {
			return nil, errors.Errorf("keypairs[%d]: parse peer id", nki)
		}

		objKey := NewKeypairKey(pid.Pretty())
		kpObjectKeys[nki] = objKey
		if _, dupe := createdKp[objKey]; dupe {
			return nil, errors.Errorf("keypairs[%d]: duplicate: %s", nki, pid.Pretty())
		}
		createdKp[objKey] = struct{}{}
	}
	for _, kp := range kps {
		// store keypair
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

	// create / validate the objectref
	var kp *identity.Keypair
	kp, err = FollowKeypair(ctx, worldHandle.AccessWorldState, kpRef)
	if err == nil && kp.GetPeerId() == "" {
		err = errors.New("keypair cannot be empty")
	}
	if err != nil {
		return false, err
	}

	if err := kp.Validate(); err != nil {
		return false, err
	}

	pid, err := kp.ParsePeerID()
	if err != nil {
		return false, err
	}

	pidPretty := pid.Pretty()
	objKey := NewKeypairKey(pidPretty)

	// create the object if it doesn't exist.
	obj, objFound, err := worldHandle.GetObject(objKey)
	if err != nil {
		return false, err
	}
	if objFound {
		_, err = obj.SetRootRef(kpRef)
		return false, err
	}

	obj, err = worldHandle.CreateObject(objKey, kpRef)
	if err != nil {
		return false, err
	}

	// set keypair type ref
	typesState := world_types.NewTypesState(ctx, worldHandle)
	if err := typesState.SetObjectType(objKey, KeypairTypeID); err != nil {
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
	// Applying to an existing object.
	keypairRef := o.GetKeypairRef()
	_, err = FollowKeypair(ctx, objectHandle.AccessWorldState, keypairRef)
	if err != nil {
		return false, err
	}

	// update the object
	_, err = objectHandle.SetRootRef(keypairRef)
	return false, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *KeypairUpdateOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *KeypairUpdateOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
}

// _ is a type assertion
var _ world.Operation = ((*KeypairUpdateOp)(nil))
