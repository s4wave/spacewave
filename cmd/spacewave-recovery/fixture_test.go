//go:build !js && !wasip1

package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_tx "github.com/s4wave/spacewave/db/world/block/tx"
	recovery "github.com/s4wave/spacewave/db/world/recovery"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// fixture is an entirely synthetic persisted Volume capture and its independently known units.
type fixture struct {
	// source declares the synthetic captured contexts and native evidence locations.
	source *recovery.Source
	// blocks counts distinct content references before duplicate table insertion.
	blocks uint64
	// size sums the distinct stored block bytes.
	size uint64
	// liveChild is omitted from its inherited unverified list but present in typed bytes.
	liveChild *block.BlockRef
	// liveBody is the current live object's exact version.
	liveBody *bucket.ObjectRef
	// refused is the typed body's exact surviving version.
	refused *bucket.ObjectRef
	// expected is derived from native writer child refs before corrupting capture lists.
	expected *recovery.Counts
}

// makeFixture builds native Worlds and persists their ordinary Volume records in SQLite.
func makeFixture(t *testing.T) *fixture { return makeFixtureWithConflict(t, false) }

// makeFixtureWithConflict optionally gives one exact version two native historical types.
func makeFixtureWithConflict(t *testing.T, conflict bool) *fixture {
	// Keep all fixture construction in isolated temporary storage.
	t.Helper()
	ctx := t.Context()
	store := store_inmem.NewStore()
	keygen := store_kvkey.NewDefaultKVKey()
	blocks, err := volume_kvtx.NewVolume(ctx, "synthetic-volume", keygen, store, nil, false, true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocks.Close()
	put := func(value block.Block) *block.BlockRef {
		t.Helper()
		data, err := value.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		refs, err := block.ExtractBlockRefs(value)
		if err != nil {
			t.Fatal(err)
		}
		ref, _, err := blocks.PutBlock(ctx, data, &block.PutOpts{Refs: refs, RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	raw := func(data []byte) *block.BlockRef {
		t.Helper()
		ref, _, err := blocks.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	liveChild := raw([]byte("abc"))
	missing, err := block.BuildBlockRef([]byte("xyz"), nil)
	if err != nil {
		t.Fatal(err)
	}
	body := func(label string, child *block.BlockRef) *bucket.ObjectRef {
		value := &blob.Blob{TotalSize: uint64(len(label)), RawData: []byte(label)}
		if child != nil {
			value = &blob.Blob{BlobType: blob.BlobType_BlobType_CHUNKED, TotalSize: 3, ChunkIndex: &blob.ChunkIndex{Chunks: []*blob.Chunk{{DataRef: child, Size: 3}}}}
		}
		return &bucket.ObjectRef{RootRef: put(&kvtx_block.KeyValueStore{ImplType: kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL, IavlRoot: &iavl.Node{Key: []byte("item"), Size: 1, ValueBlob: value}})}
	}

	// Use the actual legacy IAVL World reader/writer, with real type graph edges.
	initial := world_block.NewWorld(world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV, false)
	initial.ObjectKeyValue = kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL)
	initial.GraphKeyValue = kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL)
	start := &bucket.ObjectRef{RootRef: put(initial), BucketId: "synthetic-space"}
	engine, err := world_block.OpenSnapshot(ctx, logrus.NewEntry(logrus.New()), blocks, start)
	if err != nil {
		t.Fatal(err)
	}
	create := func(engine *world_block.Engine, key string, root *bucket.ObjectRef, typed bool) {
		t.Helper()
		ws := world.NewEngineWorldState(engine, true)
		object, err := ws.CreateObject(ctx, key, root)
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
		if typed {
			if err := world_types.SetObjectType(ctx, ws, key, "kv/store"); err != nil {
				t.Fatal(err)
			}
		}
	}
	liveBody := body("live", liveChild)
	create(engine, "live", liveBody, true)
	create(engine, "intentional", body("intentional", nil), true)
	before := engine.GetRootRef().CloneVT()
	refusedBody := body("surviving refused content", nil)
	create(engine, "refused", refusedBody, true)
	accepted := engine.GetRootRef().CloneVT()
	create(engine, "partial", body("partial", missing), true)
	unknownBody := raw([]byte("unavailable body type"))
	create(engine, "unknown-type", &bucket.ObjectRef{RootRef: unknownBody}, false)
	if conflict {
		if err := world_types.SetObjectType(ctx, world.NewEngineWorldState(engine, true), "refused", "sync/app/recovery-fixture"); err != nil {
			t.Fatal(err)
		}
	}
	historical := engine.GetRootRef().CloneVT()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	// Replay the divergent checkpoint from before the accepted refused operation.
	currentEngine, err := world_block.OpenSnapshot(ctx, logrus.NewEntry(logrus.New()), blocks, before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := world.NewEngineWorldState(currentEngine, true).DeleteObject(ctx, "intentional"); err != nil {
		t.Fatal(err)
	}
	current := currentEngine.GetRootRef().CloneVT()
	if err := currentEngine.Close(); err != nil {
		t.Fatal(err)
	}
	raw([]byte("unassigned synthetic payload"))
	put(&world_block.Object{Key: "unbound", RootRef: body("unbound", nil), Rev: 1})

	// Persist genuine replay/operation/checkpoint messages, with an exact author entry.
	operationEffect := &space_world.WorldOp{Body: &space_world.WorldOp_ApplyTxOp{ApplyTxOp: &space_world.ApplyTxOp{Tx: &world_tx.Tx{TxType: world_tx.TxType_TxType_CREATE_OBJECT, TxCreateObject: &world_tx.TxCreateObject{ObjectKey: "refused", RootRef: refusedBody}}}}}
	effect, err := operationEffect.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	private, _, err := crypto.GenerateEd25519Key(bytes.NewReader(bytes.Repeat([]byte{8}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	author, err := peer.IDFromPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	configHash := sha256.Sum256([]byte("synthetic-config"))
	operation := &sobject.SOOperationInner{PeerId: author.String(), LocalId: "01arz3ndektsv4rrffq69g5fav", Nonce: 1, OpData: effect, SharedObjectId: "synthetic-space", ConfigHash: configHash[:], ProtocolVersion: sobject.SOOperationProtocolVersion}
	if err := operation.Validate(); err != nil {
		t.Fatal(err)
	}
	opData, err := operation.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	opHash := sobject.HashSOOperationInner(opData)
	acceptedCursor := &space_world.ReplayCursor{Base: &space_world.InnerState{HeadRef: before}, Head: &space_world.InnerState{HeadRef: accepted}, Outcomes: []*space_world.ReplayCursorOutcome{{Hash: opHash, World: accepted.GetRootRef()}}}
	refusedCursor := &space_world.ReplayCursor{Base: &space_world.InnerState{HeadRef: before}, Head: &space_world.InnerState{HeadRef: before}, Outcomes: []*space_world.ReplayCursorOutcome{{Hash: opHash, World: before.GetRootRef(), Reason: "synthetic handler refusal"}}}
	stateData, err := (&space_world.InnerState{HeadRef: before}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := &sobject.SOCheckpointInner{SharedObjectId: "synthetic-space", Height: 1, ConfigHash: operation.GetConfigHash(), StateData: stateData, Authors: []*sobject.SOOperationPosition{{PeerId: operation.GetPeerId(), Nonce: 1, OpHash: opHash}}, PrevCheckpointHash: make([]byte, 32), ReplayVersion: sobject.SOReplayVersion}
	if err := checkpoint.Validate(); err != nil {
		t.Fatal(err)
	}

	// Write plain SQLite rows without a Volume opener, with duplicate physical locations.
	path := filepath.Join(t.TempDir(), "capture.sqlite")
	db, err := sql.Open(sqliteDriver(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"volume", "duplicate"} {
		if _, err := db.Exec("CREATE TABLE " + table + " (key BLOB PRIMARY KEY,value BLOB NOT NULL)"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO "+table+" VALUES (?,?)", keygen.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(9)); err != nil {
			t.Fatal(err)
		}
	}
	result := &fixture{liveChild: liveChild, liveBody: liveBody, refused: refusedBody}
	read, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	err = read.ScanPrefix(ctx, keygen.GetBlockFullPrefix(), func(key, value []byte) error {
		result.blocks++
		result.size += uint64(len(value))
		for _, table := range []string{"volume", "duplicate"} {
			if _, err := db.Exec("INSERT INTO "+table+" VALUES (?,?)", key, value); err != nil {
				return err
			}
		}
		return nil
	})
	read.Discard()
	if err != nil {
		t.Fatal(err)
	}
	insert := func(key []byte, value block.Block) {
		t.Helper()
		data, err := value.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO volume VALUES (?,?)", key, data); err != nil {
			t.Fatal(err)
		}
	}
	insert([]byte("native/accepted"), acceptedCursor)
	insert([]byte("native/refused"), refusedCursor)
	insert([]byte("native/operation"), operation)
	insert([]byte("native/checkpoint"), checkpoint)
	rootKey := append([]byte{2, 'R'}, binary.AppendUvarint(nil, uint64(len("synthetic-space")))...)
	rootKey = append(rootKey, []byte("synthetic-space/current")...)
	insert(rootKey, &block_gc.RootEntry{Ref: block_gc.BlockKey(current.GetRootRef())})
	listKey := append([]byte{2, 'c'}, block_gc.BlockKey(liveBody.GetRootRef())...)
	if _, err := db.Exec("INSERT INTO volume VALUES (?,?)", listKey, block_gc.EncodeChildren(nil)); err != nil {
		t.Fatal(err)
	}
	insert([]byte{2, 'S'}, &block_gc.LivenessState{UnverifiedLists: true})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	result.source = &recovery.Source{CaptureId: "synthetic-b8", Path: path, Tables: []string{"volume", "duplicate"}, Worlds: []*recovery.WorldInput{
		{SpaceId: "synthetic-space", Ref: before}, {SpaceId: "synthetic-space", Ref: accepted}, {SpaceId: "synthetic-space", Ref: historical}, {SpaceId: "synthetic-space", Ref: current, Current: true},
	}, Witnesses: []*recovery.WitnessInput{{SpaceId: "synthetic-space", AcceptedKey: []byte("native/accepted"), RefusedKey: []byte("native/refused"), OperationKey: []byte("native/operation"), CheckpointKey: []byte("native/checkpoint")}}}
	// Derive full expected counts from the native writer's graph, before any scan.
	walk := func(roots []*block.BlockRef) map[string]bool {
		seen := map[string]bool{}
		pending := append([]*block.BlockRef(nil), roots...)
		for len(pending) != 0 {
			ref := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if ref.GetEmpty() || seen[ref.MarshalString()] {
				continue
			}
			stored, err := blocks.GetStoredBlock(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			if stored == nil {
				continue
			}
			if !stored.GetRefsKnown() {
				t.Fatal("fixture writer did not record native refs")
			}
			seen[ref.MarshalString()] = true
			pending = append(pending, stored.GetRefs()...)
		}
		return seen
	}
	held := walk([]*block.BlockRef{current.GetRootRef()})
	typed := walk([]*block.BlockRef{before.GetRootRef(), accepted.GetRootRef(), historical.GetRootRef(), current.GetRootRef()})
	delete(typed, unknownBody.MarshalString())
	expected := &recovery.Counts{PhysicalLocations: result.blocks * 2, PhysicalBlocks: result.blocks, PhysicalBytes: result.size, ObjectVersions: 6, CandidateKeys: 4, AttributableVersions: 4, RecoverableVersions: 2, PartialVersions: 2, ConfirmedRefusalLosses: 1, IntentionalDeletes: 1, StructuralCandidates: 1, UnassignedBlocks: result.blocks - uint64(len(typed))}
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.ScanPrefix(ctx, keygen.GetBlockFullPrefix(), func(key, value []byte) error {
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(key[len(keygen.GetBlockFullPrefix()):]); err != nil {
			return err
		}
		if !held[ref.MarshalString()] {
			expected.OrphanBlocks++
			expected.OrphanBytes += uint64(len(value))
		}
		return nil
	})
	tx.Discard()
	if err != nil {
		t.Fatal(err)
	}
	result.expected = expected
	return result
}

// sqliteDriver names the registered SQLite driver of this build.
func sqliteDriver() string {
	if slices.Contains(sql.Drivers(), "sqlite") {
		return "sqlite"
	}
	return "sqlite3"
}

// writeManifest writes the exact binary Source consumed by the production command.
func writeManifest(t *testing.T, source *recovery.Source) string {
	t.Helper()
	data, err := source.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.pb")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
