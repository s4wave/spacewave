package provider_local

import (
	"context"
	"crypto/rand"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// SharedObject implements the sobject interface attached to sobjectTracker.
type SharedObject struct {
	ctx context.Context
	tkr *sobjectTracker

	blkStore  bstore.BlockStore
	soHost    *sobject.SOHost
	lsoHost   *LocalSOHost
	objStore  object.ObjectStore
	localPriv crypto.PrivKey
	localPid  peer.ID
}

// GetSOHostState returns a snapshot of the current SOState via the SOHost.
func (s *SharedObject) GetSOHostState(ctx context.Context) (*sobject.SOState, error) {
	return s.soHost.GetHostState(ctx)
}

// GetBus returns the bus used for the shared object.
func (s *SharedObject) GetBus() bus.Bus {
	return s.tkr.a.t.p.b
}

// GetPeerID returns the local peer id for the shared object.
func (s *SharedObject) GetPeerID() peer.ID {
	return s.localPid
}

// GetSharedObjectID returns the shared object id.
func (s *SharedObject) GetSharedObjectID() string {
	return s.tkr.id
}

// GetBlockStore returns the block store mounted along with the SharedObject.
func (s *SharedObject) GetBlockStore() bstore.BlockStore {
	return s.blkStore
}

// AccessLocalStateStore accesses a kvtx ops for a local state store with the given ID.
// This state store is stored along with the local SharedObject state.
func (s *SharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	// ls = local state
	storePrefix := []byte("ls/")
	prefixedObjStore := object.NewPrefixer(s.objStore, storePrefix)
	relReleased := context.AfterFunc(s.ctx, released)
	return prefixedObjStore, func() { relReleased() }, nil
}

// GetSharedObjectState returns an snapshot of the shared object state.
func (s *SharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	stateCtr, relStateCtr, err := s.lsoHost.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relStateCtr()

	val, err := stateCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return val, nil
}

// AccessSharedObjectState adds a reference to the state and returns the state container.
// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
func (s *SharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return s.lsoHost.AccessSharedObjectState(ctx, released)
}

// QueueOperation applies an operation to the shared object op queue.
// Returns after the operation is applied to the local queue.
// Returns the local operation ID.
func (s *SharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	return s.lsoHost.QueueOperation(ctx, op)
}

// WaitOperation waits for the operation to be confirmed or rejected by the provider.
// Returns the current state nonce (greater than or equal to the nonce when the op was applied).
// After ClearOperation has been called, this will return success even for failed ops!
// If the operation was rejected, returns 0, true, error.
// Any other error returns 0, false, error
func (s *SharedObject) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	return s.lsoHost.WaitOperation(ctx, localID)
}

// ClearOperationResult clears the operation state (rejection).
// No-op if the operation was successfully applied.
// Be sure to call this after WaitOperation returns an error.
// Call with the local operation id.
func (s *SharedObject) ClearOperationResult(ctx context.Context, localID string) error {
	// Clearing the rejected operations happens in the Execute loop.
	// Here, we can just clear the locally stored op state.
	return s.lsoHost.clearLocalOpResult(ctx, localID)
}

// ProcessOperations processes operations as a validator.
// The ops should be processed in the order they are provided.
// The results must be a subset of ops (but does not need to have all ops).
// If watch is set, waits for ops to be queued, then calls cb. Does not return.
// If watch is unset, if there are no available ops, returns immediately.
// cb is called with the state snapshot and the decoded inner state.
func (s *SharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	// Get the state container
	stateCtr, relStateCtr, err := s.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	var current *sobject.SOState
	for {
		// Wait for state
		next, err := stateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next

		// Get pending operations
		pendingOps := current.GetOps()
		if len(pendingOps) == 0 {
			if !watch {
				return nil
			}
			continue
		}

		// Create state snapshot
		snap := sobject.NewSOStateParticipantHandle(
			s.tkr.a.le,
			s.tkr.a.t.p.sfs,
			s.GetSharedObjectID(),
			current,
			s.localPriv,
			s.localPid,
		)

		// Process the operations through the snapshot
		nextRoot, rejectedOps, acceptedOps, err := snap.ProcessOperations(
			ctx,
			pendingOps,
			func(ctx context.Context, currentStateData []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
				return cb(ctx, snap, currentStateData, ops)
			},
		)
		if err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			if !watch {
				return err
			}
			s.tkr.a.le.WithError(err).Warn("error processing operations")
			continue
		}

		// Update the state
		if err := s.soHost.UpdateRootState(
			ctx,
			nextRoot,
			s.GetPeerID().String(),
			rejectedOps,
			acceptedOps,
		); err != nil {
			return err
		}

		if !watch {
			return nil
		}
	}
}

// sobjectTracker tracks a SharedObject in the ProviderAccount.
type sobjectTracker struct {
	// a is the provider account
	a *ProviderAccount
	// id is the sobject id
	id string
	// ref is the reference to the shared object
	// set when instantiating the tracker
	ref *promise.Promise[*sobject.SharedObjectRef]
	// sobjectProm is the sobject promise container
	sobjectProm *promise.PromiseContainer[*SharedObject]
	// healthCtr contains the current shared object health snapshot.
	healthCtr *ccontainer.CContainer[*sobject.SharedObjectHealth]
}

// buildSharedObjectTracker builds a new sobjectTracker for a sobject id.
func (a *ProviderAccount) buildSharedObjectTracker(sobjectID string) (keyed.Routine, *sobjectTracker) {
	tracker := &sobjectTracker{
		a:           a,
		id:          sobjectID,
		ref:         promise.NewPromise[*sobject.SharedObjectRef](),
		sobjectProm: promise.NewPromiseContainer[*SharedObject](),
		healthCtr: ccontainer.NewCContainer[*sobject.SharedObjectHealth](
			sobject.NewSharedObjectLoadingHealth(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			),
		),
	}
	return tracker.executeSharedObjectTracker, tracker
}

// setHealth updates the current shared object health snapshot when tracking is enabled.
func (t *sobjectTracker) setHealth(health *sobject.SharedObjectHealth) {
	if t.healthCtr == nil {
		return
	}
	t.healthCtr.SetValue(health)
}

// executeSharedObjectTracker exeecutes the sobjectTracker for the sobject.
func (t *sobjectTracker) executeSharedObjectTracker(rctx context.Context) (rerr error) {
	// clear old state if any
	t.sobjectProm.SetPromise(nil)
	t.setHealth(
		sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
	)
	defer func() {
		if rerr != nil && rerr != context.Canceled {
			t.setHealth(sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				rerr,
			))
			t.sobjectProm.SetResult(nil, rerr)
		}
	}()

	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	le := t.a.le.WithField("sobject-id", t.id)
	le.Debug("mounting sobject")

	// Wait for the ref
	sobjectRef, err := t.ref.Await(ctx)
	if err != nil {
		return err
	}

	provRef := sobjectRef.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()
	sharedObjectID := provRef.GetId()

	// Mount block store
	blkStore, blkStoreRef, err := bstore.ExMountBlockStore(
		ctx,
		t.a.t.p.b,
		NewBlockStoreRef(
			providerID,
			providerAccountID,
			sobjectRef.GetBlockStoreId(),
		),
		false,
		ctxCancel,
	)
	if err != nil {
		return err
	}
	defer blkStoreRef.Release()

	le.Debug("mounted block store for sobject successfully")

	// Create and/or open the object store in the account volume.
	volID := t.a.vol.GetID()
	objectStoreID := SobjectObjectStoreID(
		providerID,
		providerAccountID,
	)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		t.a.t.p.b,
		false,
		objectStoreID,
		volID,
		ctxCancel,
	)
	if err != nil {
		return err
	}
	defer diRef.Release()

	le.Debug("mounted object store for sobject successfully")

	// Get the peer id from the volume for ops.
	localPeer, err := t.a.vol.GetPeer(ctx, true)
	if err != nil {
		return err
	}
	localPeerID := localPeer.GetPeerID()
	localPeerIDStr := localPeerID.String()

	// Get the local priv key
	localPriv, err := localPeer.GetPrivKey(ctx)
	if err != nil {
		return err
	}

	// Write the initial state if not found.
	objStore := objStoreHandle.GetObjectStore()
	objStoreKey := SobjectObjectStoreHostStateKey(sharedObjectID)
	if err := t.initSharedObjectState(
		ctx,
		le,
		objStore,
		objStoreKey,
		sharedObjectID,
		localPeerIDStr,
		localPriv,
	); err != nil {
		return err
	}

	// Construct the shared object state handle.
	// Since this is the "local" provider we can "lock" the state with an in-memory lock.
	watchFn, lockFn := NewObjectStoreSOStateFuncs(ctx, objStore)
	soHost := sobject.NewSOHost(ctx, watchFn, lockFn, sharedObjectID)

	// construct the local host logic
	lsoHost, err := NewLocalSOHost(
		le,
		localPriv,
		soHost,
		objStore,
		sharedObjectID,
		t.a.t.p.sfs,
	)
	if err != nil {
		return err
	}

	so := &SharedObject{
		ctx:       ctx,
		tkr:       t,
		blkStore:  blkStore,
		soHost:    soHost,
		lsoHost:   lsoHost,
		objStore:  objStore,
		localPriv: localPriv,
		localPid:  localPeerID,
	}
	t.setHealth(
		sobject.NewSharedObjectReadyHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
	)
	t.sobjectProm.SetResult(so, nil)
	defer t.sobjectProm.SetPromise(nil)

	// execute the local SOHost logic
	return lsoHost.Execute(ctx)
}

// createSharedObjectLocked creates a new sobject with the given details.
// Assumes p.mtx is locked.
func (a *ProviderAccount) createSharedObjectLocked(ctx context.Context, id string, meta *sobject.SharedObjectMeta) (*sobject.SharedObjectRef, error) {
	// build the sobject ref
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()

	// Create the shared object and block store ref.
	blockStoreID := SobjectBlockStoreID(id)
	sobjectRef := sobject.NewSharedObjectRef(providerID, providerAccountID, id, blockStoreID)
	if err := sobjectRef.Validate(); err != nil {
		return nil, err
	}

	// validate meta
	if err := meta.Validate(); err != nil {
		return nil, err
	}

	// Get the current object list.
	sharedObjectList := a.soListCtr.GetValue().CloneVT()
	if sharedObjectList == nil {
		sharedObjectList = &sobject.SharedObjectList{}
	}

	// Check the shared object id does not already exist.
	for _, soListEntry := range sharedObjectList.GetSharedObjects() {
		if soListEntry.GetRef().GetProviderResourceRef().GetId() == id {
			return nil, sobject.ErrSharedObjectExists
		}
	}

	// create the block store first
	bstoreRef, err := a.createBlockStoreLocked(ctx, blockStoreID)
	if err != nil {
		return nil, err
	}
	_ = bstoreRef

	// Register GC hierarchy: gcroot -> provider -> bucket
	if kvVol, ok := a.vol.(kvtx_volume.KvtxVolume); ok {
		if rg := kvVol.GetRefGraph(); rg != nil {
			bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)
			if err := block_gc.RegisterEntityChain(ctx, rg,
				block_gc.NodeGCRoot,
				ProviderIRI(providerID),
				block_gc.BucketIRI(bucketID),
			); err != nil {
				return nil, err
			}
		}
	}

	// Append to the list of shared objects.
	sharedObjectList.SharedObjects = append(sharedObjectList.SharedObjects, &sobject.SharedObjectListEntry{
		Ref:  sobjectRef.CloneVT(),
		Meta: meta.CloneVT(),
	})
	slices.SortFunc(sharedObjectList.SharedObjects, func(a, b *sobject.SharedObjectListEntry) int {
		return strings.Compare(a.GetRef().GetProviderResourceRef().GetId(), b.GetRef().GetProviderResourceRef().GetId())
	})

	// Write the list
	if err := a.writeSharedObjectList(ctx, sharedObjectList); err != nil {
		return nil, err
	}

	// Update the list
	a.soListCtr.SetValue(sharedObjectList)

	// return the shared object ref
	return sobjectRef, nil
}

// CreateSharedObject creates a new sobject with the given details.
func (a *ProviderAccount) CreateSharedObject(ctx context.Context, id string, meta *sobject.SharedObjectMeta, _, _ string) (*sobject.SharedObjectRef, error) {
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer relMtx()

	return a.createSharedObjectLocked(ctx, id, meta)
}

// UpdateSharedObjectMeta updates the metadata for an existing shared object.
func (a *ProviderAccount) UpdateSharedObjectMeta(ctx context.Context, id string, meta *sobject.SharedObjectMeta) error {
	if err := meta.Validate(); err != nil {
		return err
	}

	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer relMtx()

	sharedObjectList := a.soListCtr.GetValue().CloneVT()
	if sharedObjectList == nil {
		return sobject.ErrSharedObjectNotFound
	}

	idx := slices.IndexFunc(sharedObjectList.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == id
	})
	if idx == -1 {
		return sobject.ErrSharedObjectNotFound
	}

	sharedObjectList.SharedObjects[idx].Meta = meta.CloneVT()
	if err := a.writeSharedObjectList(ctx, sharedObjectList); err != nil {
		return err
	}
	a.soListCtr.SetValue(sharedObjectList)
	return nil
}

// MountSharedObject attempts to mount a SharedObject returning the sobject and a release function.
//
// usually called by the provider controller
func (a *ProviderAccount) MountSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, released func()) (sobject.SharedObject, func(), error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

	sobjectID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.sobjects.AddKeyRef(sobjectID)

	// Set the ref in the tracker if not set
	tkr.ref.SetResult(ref, nil)

	// Await the sobject handle to be ready
	ws, err := tkr.sobjectProm.Await(ctx)
	if err != nil {
		tkrRef.Release()
		return nil, nil, err
	}

	return ws, tkrRef.Release, nil
}

// AccessSharedObjectHealth adds a reference to shared object health by ref.
func (a *ProviderAccount) AccessSharedObjectHealth(
	ctx context.Context,
	ref *sobject.SharedObjectRef,
	released func(),
) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}
	sobjectID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.sobjects.AddKeyRef(sobjectID)
	tkr.ref.SetResult(ref, nil)
	return tkr.healthCtr, func() {
		tkrRef.Release()
		if released != nil {
			released()
		}
	}, nil
}

// AccessSharedObjectList adds a reference to the list of shared objects and returns the container.
// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
func (a *ProviderAccount) AccessSharedObjectList(ctx context.Context, released func()) (ccontainer.Watchable[*sobject.SharedObjectList], func(), error) {
	return a.soListCtr, func() {}, nil
}

// initSharedObjectState initializes or loads the shared object state from the object store.
func (t *sobjectTracker) initSharedObjectState(
	ctx context.Context,
	le *logrus.Entry,
	objStore object.ObjectStore,
	objStoreKey []byte,
	sharedObjectID string,
	localPeerIDStr string,
	localPriv crypto.PrivKey,
) error {
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, objStoreKey)
	if err != nil {
		return err
	}

	val := &sobject.SOState{}
	if found {
		if err := val.UnmarshalVT(data); err != nil {
			return err
		}
		if err := val.Validate(sharedObjectID); err != nil {
			return err
		}
	} else {
		le.Debug("initializing shared object with empty state")
		val.Config = &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: localPeerIDStr,
				Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
			}},
		}

		// TODO move to common functions(!)
		ninner := &sobject.SORootInner{
			Seqno:     1,
			StateData: nil, // TODO
		}

		// generate random transform config
		encKey := make([]byte, 32)
		_, err = rand.Read(encKey)
		if err != nil {
			return err
		}

		soTransformConf, err := block_transform.NewConfig([]config.Config{
			&transform_blockenc.Config{
				BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
				Key:      encKey,
			},
		})
		if err != nil {
			return err
		}

		soTransform, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, t.a.t.p.sfs, soTransformConf)
		if err != nil {
			return err
		}

		innerDataDec, err := ninner.MarshalVT()
		if err != nil {
			return err
		}

		innerDataEnc, err := soTransform.EncodeBlock(innerDataDec)
		if err != nil {
			return err
		}

		nroot := &sobject.SORoot{InnerSeqno: 1, Inner: innerDataEnc}
		if err := nroot.SignInnerData(localPriv, sharedObjectID, nroot.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
			return err
		}
		val.Root = nroot

		// Make a grant for each of the remote peers.
		participants := val.GetConfig().GetParticipants()
		grantToPeerIDs := make([]string, 0, len(participants))
		for _, participant := range participants {
			if sobject.CanReadState(participant.GetRole()) {
				grantToPeerIDs = append(grantToPeerIDs, participant.GetPeerId())
			}
		}

		grants := make([]*sobject.SOGrant, len(grantToPeerIDs))
		nextGrantInner := &sobject.SOGrantInner{TransformConf: soTransformConf}
		for i, grantPeerIDStr := range grantToPeerIDs {
			grantPeerID, err := peer.IDB58Decode(grantPeerIDStr)
			if err != nil {
				return errors.Wrapf(err, "participants[%d]: invalid participant peer id", i)
			}
			grantPub, err := grantPeerID.ExtractPublicKey()
			if err != nil {
				return errors.Wrapf(err, "participants[%d]: invalid participant peer pub: %s", i, grantPeerID.String())
			}
			grant, err := sobject.EncryptSOGrant(localPriv, grantPub, sharedObjectID, nextGrantInner)
			if err != nil {
				return err
			}
			grants[i] = grant
		}
		val.RootGrants = grants

		if err := val.Validate(sharedObjectID); err != nil {
			return err
		}

		data, err = val.MarshalVT()
		if err != nil {
			return err
		}

		if err := otx.Set(ctx, objStoreKey, data); err != nil {
			return err
		}

		if err := otx.Commit(ctx); err != nil {
			return err
		}
	}

	return nil
}

// buildSoObjectStore builds the shared object store for the provider account.
func (a *ProviderAccount) buildSoObjectStore(ctx context.Context) (object.ObjectStore, func(), error) {
	// Get the object store ID
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()
	objectStoreID := SobjectObjectStoreID(providerID, providerAccountID)

	// Look up the object store
	volID := a.vol.GetID()
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		a.t.p.b,
		false,
		objectStoreID,
		volID,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return objStoreHandle.GetObjectStore(), diRef.Release, nil
}

// readSharedObjectList reads and returns the shared object list from storage.
func (a *ProviderAccount) readSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error) {
	objStore, release, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Create object store transaction
	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	// Read the list
	data, found, err := otx.Get(ctx, SobjectObjectStoreListKey())
	if err != nil {
		return nil, err
	}

	list := &sobject.SharedObjectList{}
	if found {
		if err := list.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}

	return list, nil
}

// writeSharedObjectList writes the shared object list to storage.
func (a *ProviderAccount) writeSharedObjectList(ctx context.Context, list *sobject.SharedObjectList) error {
	objStore, release, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Create object store transaction
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	// Marshal and write the list
	data, err := list.MarshalVT()
	if err != nil {
		return err
	}

	if err := otx.Set(ctx, SobjectObjectStoreListKey(), data); err != nil {
		return err
	}

	return otx.Commit(ctx)
}

// GetSOHost returns the SOHost for invite operations.
func (s *SharedObject) GetSOHost() *sobject.SOHost {
	return s.soHost
}

// GetPrivKey returns the private key for signing invite messages.
func (s *SharedObject) GetPrivKey() crypto.PrivKey {
	return s.localPriv
}

// GetProviderID returns the provider identifier for the invite message.
func (s *SharedObject) GetProviderID() string {
	return s.tkr.a.t.accountInfo.GetProviderId()
}

// CreateSOInviteOp creates a signed invite and stores it locally.
func (s *SharedObject) CreateSOInviteOp(
	ctx context.Context,
	ownerPrivKey crypto.PrivKey,
	role sobject.SOParticipantRole,
	providerID string,
	targetPeerID string,
	maxUses uint32,
	expiresAt *timestamppb.Timestamp,
) (*sobject.SOInviteMessage, error) {
	return s.soHost.CreateSOInviteOp(
		ctx,
		ownerPrivKey,
		role,
		providerID,
		targetPeerID,
		maxUses,
		expiresAt,
	)
}

// RevokeInvite revokes an invite locally.
func (s *SharedObject) RevokeInvite(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error {
	return s.soHost.RevokeInvite(ctx, signerPrivKey, inviteID)
}

// IncrementInviteUses increments invite uses locally.
func (s *SharedObject) IncrementInviteUses(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error {
	return s.soHost.IncrementInviteUses(ctx, signerPrivKey, inviteID)
}

// _ is a type assertion
var (
	_ sobject.SharedObjectProvider      = ((*ProviderAccount)(nil))
	_ sobject.SharedObject              = ((*SharedObject)(nil))
	_ sobject.InviteHost                = ((*SharedObject)(nil))
	_ sobject.SharedObjectStateSnapshot = (*lsoStateSnapshot)(nil)
)
