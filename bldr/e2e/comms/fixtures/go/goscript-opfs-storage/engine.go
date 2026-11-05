//go:build goscript

package goscript_opfs_storage

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	browser_storage "github.com/s4wave/spacewave/bldr/storage/browser"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	volume_idb "github.com/s4wave/spacewave/db/volume/idb"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// publicationStore holds public fixture references across the worker restart.
const publicationStore = "browser-publication-proof"

// checkEngineGraph drives the real Space producer before the worker restart,
// then serves its persisted advertisement from the reopened account volume.
func checkEngineGraph(ctx context.Context, name string, write bool) error {
	// Open the fixture's metadata volume through its real browser controller.
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), db_testbed.WithVolumeConfig(&volume_browser.Config{Name: name}))
	if err != nil {
		return err
	}
	defer tb.Release()

	// Keep ephemeral fixture references in existing protobuf messages.
	store, release, err := tb.Volume.AccessObjectStore(ctx, publicationStore, nil)
	if err != nil {
		return err
	}
	defer release()
	ref := &sobject.SharedObjectRef{}
	head := &bucket.ObjectRef{}
	if write {
		ref, head, err = produceEngineGraph(ctx, tb, name)
		if err != nil {
			return err
		}
	}

	// Store or reload the actual advertised graph and the account that owns it.
	tx, err := store.NewTransaction(ctx, write)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, item := range []struct {
		// key isolates the existing protobuf message within fixture metadata.
		key string
		// message holds the public reference needed after the worker restart.
		message interface {
			MarshalVT() ([]byte, error)
			UnmarshalVT([]byte) error
		}
	}{{"engine-space", ref}, {"engine-advertisement", head}} {
		if write {
			data, err := item.message.MarshalVT()
			if err != nil {
				return err
			}
			if err := tx.Set(ctx, []byte(item.key), data); err != nil {
				return err
			}
		} else {
			data, found, err := tx.Get(ctx, []byte(item.key))
			if err != nil {
				return err
			}
			if !found {
				return errors.Errorf("missing fixture reference %s", item.key)
			}
			if err := item.message.UnmarshalVT(data); err != nil {
				return err
			}
		}
	}
	if write {
		return tx.Commit(ctx)
	}
	tx.Discard()

	// Reopen the producer's real account volume after the worker was terminated.
	accountName := "volumes/" + name + "/engine/" + provider_local.StorageVolumeID("local", ref.GetProviderResourceRef().GetProviderAccountId())
	account, err := db_testbed.NewTestbed(ctx, tb.Logger, db_testbed.WithVolumeConfig(&volume_browser.Config{Name: accountName}))
	if err != nil {
		return err
	}
	defer account.Release()
	conf, err := account.Volume.GetBucketConfig(ctx, head.GetBucketId())
	if err != nil {
		return err
	}
	if conf == nil || conf.GetId() != head.GetBucketId() {
		return errors.New("reopened fixture volume lacks its persisted account bucket")
	}
	if err := walkEngineGraph(ctx, account, head, "reopened checkpoint"); err != nil {
		return err
	}

	// Close the account's handles before deleting this owned fixture volume.
	account.Release()
	return volume_browser.Delete(accountName)
}

// produceEngineGraph uses public provider, Session, Space and sequencer APIs;
// only the known device choice and fixture namespace differ from the app.
func produceEngineGraph(ctx context.Context, tb *db_testbed.Testbed, name string) (ref *sobject.SharedObjectRef, head *bucket.ObjectRef, rerr error) {
	// Add only the account storage primitives the public provider needs.
	ctx, cancel := context.WithCancel(ctx)
	var accountVolume volume.Volume
	defer func() {
		// Every later-deferred body, provider and storage reference releases first.
		cancel()
		if accountVolume != nil {
			if err := accountVolume.Close(); rerr == nil {
				rerr = err
			}
		}
	}()

	// Retain the browser storage controller until its account holders release.
	storages := browser_storage.BuildStorage(tb.Bus, name+"/engine/")
	tb.StaticResolver.AddFactory(storage_volume.NewFactory(tb.Bus))
	storage := storage_controller.BuildStorageController("default", storages, controller.NewInfo("test/browser-engine-storage", controller.MustParseVersion("0.0.1"), "browser engine storage"))
	releaseStorage, err := tb.Bus.AddController(ctx, storage, nil)
	if err != nil {
		return nil, nil, err
	}
	defer releaseStorage()

	// Retain the public provider and Space body factories on this bus.
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	_, controller, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{ProviderId: "local", StorageId: "default"}), nil)
	if err != nil {
		return nil, nil, err
	}
	defer controller.Release()
	p, providerRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		return nil, nil, err
	}
	defer providerRef.Release()

	// Select the account before mounting, preserving the forced-IDB variant.
	accountID := ulid.NewULID()
	if name == idbVolume {
		device, err := volume_idb.OpenDevice(ctx, "volumes/"+name+"/engine/"+provider_local.StorageVolumeID("local", accountID))
		if err != nil {
			return nil, nil, err
		}
		if err := device.Close(); err != nil {
			return nil, nil, err
		}
	}

	// Mount an independently credentialed Session on the selected account.
	account, releaseAccount, err := p.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		return nil, nil, err
	}
	defer releaseAccount()
	local := account.(*provider_local.ProviderAccount)
	sessionRef := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{ProviderId: "local", ProviderAccountId: accountID, Id: ulid.NewULID()}}
	_, releaseSession, err := local.MountSession(ctx, sessionRef, nil)
	if err != nil {
		return nil, nil, err
	}
	defer releaseSession()

	// Create and retain a real Space on that account's physical volume.
	ref, err = local.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		return nil, nil, err
	}
	so, releaseSO, err := local.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, nil, err
	}
	defer releaseSO()
	localSO := so.(*provider_local.SharedObject)
	accountVolume = localSO.GetBackingVolume()

	// Retain the actual Space body and its World engine while producing data.
	_, spaceController, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		return nil, nil, err
	}
	defer spaceController.Release()
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		return nil, nil, err
	}
	defer bodyRef.Release()
	engine := body.GetSharedObjectBody().GetWorldEngine()

	// Initialize the real filesystem object the upload writer will mutate.
	ws := world.NewEngineWorldState(engine, true)
	const objectKey = "test/browser-files"
	fsType := unixfs_world.FSType_FSType_FS_NODE
	if _, _, err := unixfs_world.FsInit(ctx, ws, "", objectKey, fsType, nil, false, time.Now()); err != nil {
		return nil, nil, err
	}

	// Exercise the resource uploads while this filesystem is still empty.
	if err := uploadEngineResource(ctx, tb, so, engine, objectKey); err != nil {
		return nil, nil, err
	}

	// Inspect the closed upload's advertisement before reading object metadata.
	if _, err := inspectEngineGraph(ctx, tb, so, "resource before object read", nil); err != nil {
		return nil, nil, err
	}

	// Require the rendered filesystem root to be an actual replay payload.
	object, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}
	defer world.ReleaseObjectState(object)
	payload, _, err := object.GetRootRef(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := inspectEngineGraph(ctx, tb, so, "resource upload payload", payload.GetRootRef()); err != nil {
		return nil, nil, err
	}

	// Retain the direct staged writer after the first-file resource path.
	writer := unixfs_world.NewBatchFSWriter(ws, objectKey, fsType, "")
	defer writer.Release()
	data := []byte("browser engine upload payload")
	if err := writer.AddFile(ctx, nil, "upload.txt", unixfs.NewFSCursorNodeType_File(), int64(len(data)), bytes.NewReader(data), 0o644, time.Now()); err != nil {
		return nil, nil, err
	}
	if err := writer.Commit(ctx); err != nil {
		return nil, nil, err
	}
	writer.Release()

	// Inspect the direct write before another object metadata snapshot.
	if _, err := inspectEngineGraph(ctx, tb, so, "direct before object read", nil); err != nil {
		return nil, nil, err
	}

	// Keep the direct writer's payload membership check after its raw walk.
	payload, _, err = object.GetRootRef(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := inspectEngineGraph(ctx, tb, so, "filesystem payload", payload.GetRootRef()); err != nil {
		return nil, nil, err
	}

	// Promote the actual derived World and wait on its checkpoint state watch.
	var derived *block.BlockRef
	if err := engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		derived = cursor.GetRef().GetRootRef().CloneVT()
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if _, err := localSO.SetSequencer(ctx, so.GetPeerID().String()); err != nil {
		return nil, nil, err
	}

	// Observe the durable checkpoint that promotes this exact derived World.
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer releaseStates()
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		// Wait until the source supplies its first checkpoint snapshot.
		if snapshot == nil {
			return false, nil
		}

		// Match the actual checkpoint root to the World just committed.
		checkpoint, err := snapshot.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		inner := &space_world.InnerState{}
		if err := inner.UnmarshalVT(checkpoint.GetStateData()); err != nil {
			return false, err
		}
		return inner.GetHeadRef().GetRootRef().EqualsRef(derived), nil
	}, nil); err != nil {
		return nil, nil, err
	}

	// Apply the existing export fence before inspecting its advertised graph.
	if err := localSO.WaitDurable(ctx); err != nil {
		return nil, nil, err
	}
	head, err = inspectEngineGraph(ctx, tb, so, "promoted checkpoint", nil)
	return ref, head, err
}

// inspectEngineGraph walks the actual PublishedGraph, storing its synthetic
// span exactly as the account copier does. It never adds the derived World.
func inspectEngineGraph(ctx context.Context, tb *db_testbed.Testbed, so sobject.SharedObject, phase string, payload *block.BlockRef) (*bucket.ObjectRef, error) {
	// Read the current immutable host snapshot before resolving its replay inputs.
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}

	// Require the expected operation payload to belong to the advertised graph.
	span, root, err := space_world.PublishedGraph(ctx, snapshot, space_world_optypes.LookupWorldOp)
	if err != nil {
		return nil, err
	}
	if !payload.GetEmpty() {
		if span == nil {
			return nil, errors.New("precheckpoint advertisement omitted the committed object payload")
		}
		decoded := &space_world.ReplaySpan{}
		if err := decoded.UnmarshalVT(span.Data); err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(decoded.GetPayloads(), payload.EqualsRef) {
			return nil, errors.New("precheckpoint advertisement omitted the committed object payload")
		}
	}

	// Store the generated span just as the real account copier does.
	if span != nil {
		if _, err := so.GetBlockStore().PutBlockBatch(ctx, []*block.PutBatchEntry{span}); err != nil {
			return nil, err
		}
	}

	// Serve that exact graph through the physical source bucket.
	head := &bucket.ObjectRef{BucketId: so.GetBlockStore().GetID(), RootRef: root}
	if err := walkEngineGraph(ctx, tb, head, phase); err != nil {
		return nil, err
	}
	return head, nil
}

// walkEngineGraph checks known child lists throughout this small fixture graph.
func walkEngineGraph(ctx context.Context, tb *db_testbed.Testbed, head *bucket.ObjectRef, phase string) error {
	// Acquire exactly the account bucket lookup used by source DEX requests.
	value, _, reference, err := bucket_lookup.ExBuildBucketLookup(ctx, tb.Bus, false, head.GetBucketId(), nil)
	if err != nil {
		return err
	}
	defer reference.Release()
	lookup, err := value.GetLookup(ctx)
	if err != nil {
		return err
	}

	// Walk every child once; this fixture has one object and a bounded graph.
	seen := make(map[string]bool)
	pending := []*block.BlockRef{head.GetRootRef()}
	for len(pending) != 0 {
		ref := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if ref.GetEmpty() || seen[ref.MarshalString()] {
			continue
		}
		seen[ref.MarshalString()] = true
		stored, err := lookup.LookupStoredBlock(ctx, ref, bucket_lookup.WithLocalOnly())
		if err != nil {
			return err
		}
		if stored == nil || !stored.RefsKnown || len(stored.Data) == 0 {
			return errors.Errorf("%s: advertised root %s misses block %s", phase, head.GetRootRef().MarshalString(), ref.MarshalString())
		}
		if err := ref.VerifyData(stored.Data, true); err != nil {
			return errors.Wrap(err, phase)
		}
		pending = append(pending, slices.Clone(stored.Refs)...)
	}
	tb.Logger.WithFields(logrus.Fields{"phase": phase, "root": head.GetRootRef().MarshalString(), "blocks": len(seen)}).Info("browser engine advertised graph complete")
	return nil
}
