package lean

import (
	"context"
	"errors"
	"sync"

	fastjson "github.com/aperturerobotics/fastjson"
	ws "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
)

// errKvNotOpen is returned when the KV API is used before KvOpen.
var errKvNotOpen = errors.New("kv: not open")

var (
	kvMtx    sync.Mutex
	kvWorld  *World
	kvStore  s4wave_kv_world.WatchKVStore
	kvCtx    context.Context
	kvCancel context.CancelFunc

	kvWatchMtx     sync.Mutex
	kvWatchCancels []context.CancelFunc
)

// KvOpen opens the embedded in-memory world and its default key/value
// store. Use KvOpenDurable for a volume that survives KvClose.
func KvOpen(ctx context.Context) error {
	return KvOpenWithWorld(ctx, func(inner context.Context) (*World, error) {
		return OpenWorld(inner)
	})
}

// KvOpenDurable opens the embedded world backed by a durable volume:
// OPFS on JavaScript targets, bbolt at dir on other platforms. State
// written through the Kv* functions survives KvClose and is visible to a
// later KvOpenDurable.
func KvOpenDurable(ctx context.Context, dir string) error {
	return KvOpenWithWorld(ctx, func(inner context.Context) (*World, error) {
		return OpenWorldDurable(inner, dir)
	})
}

// KvOpenWithWorld opens the default key/value store over a world built by
// the given constructor.
func KvOpenWithWorld(ctx context.Context, open func(context.Context) (*World, error)) error {
	kvMtx.Lock()
	defer kvMtx.Unlock()
	if kvStore != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	w, err := open(ctx)
	if err != nil {
		cancel()
		return err
	}
	store, err := s4wave_kv_world.OpenOrCreate(ctx, nil, w.WS, "sync/kv")
	if err != nil {
		w.Close()
		cancel()
		return err
	}
	kvWorld = w
	kvStore = store
	kvCtx = ctx
	kvCancel = cancel
	return nil
}

// KvClose stops all watches, closes the store, and closes the world.
func KvClose() {
	KvStopWatches()
	kvMtx.Lock()
	defer kvMtx.Unlock()
	if kvStore == nil {
		return
	}
	kvStore.Close()
	if kvWorld != nil {
		kvWorld.Close()
	}
	if kvCancel != nil {
		kvCancel()
	}
	kvStore = nil
	kvWorld = nil
	kvCtx = nil
	kvCancel = nil
}

// kvUse locks state and returns the store plus context, or errKvNotOpen.
func kvUse() (s4wave_kv_world.WatchKVStore, context.Context, error) {
	kvMtx.Lock()
	defer kvMtx.Unlock()
	if kvStore == nil {
		return nil, nil, errKvNotOpen
	}
	return kvStore, kvCtx, nil
}

// KvPut sets key to value.
func KvPut(key, value string) error {
	store, ctx, err := kvUse()
	if err != nil {
		return err
	}
	return store.Put(ctx, key, []byte(value))
}

// KvGet returns the value at key, or an empty string when absent.
func KvGet(key string) (string, error) {
	store, ctx, err := kvUse()
	if err != nil {
		return "", err
	}
	data, _, err := store.Get(ctx, key)
	return string(data), err
}

// KvExists reports whether key exists.
func KvExists(key string) (bool, error) {
	store, ctx, err := kvUse()
	if err != nil {
		return false, err
	}
	return store.Exists(ctx, key)
}

// KvDelete deletes key.
func KvDelete(key string) error {
	store, ctx, err := kvUse()
	if err != nil {
		return err
	}
	return store.Delete(ctx, key)
}

// KvList returns the entries under prefix as a JSON array of
// {"key":..., "value":...} objects.
func KvList(prefix string) (string, error) {
	store, ctx, err := kvUse()
	if err != nil {
		return "", err
	}
	entries, err := store.List(ctx, prefix)
	if err != nil {
		return "", err
	}
	return kvSnapshotJSON(entries), nil
}

// kvSnapshotJSON renders entries as a JSON array of
// {"key":..., "value":...} objects.
func kvSnapshotJSON(entries []kvtx.WatchEntry) string {
	var a fastjson.Arena
	arr := a.NewArray()
	for _, e := range entries {
		obj := a.NewObject()
		obj.Set("key", a.NewString(string(e.Key)))
		obj.Set("value", a.NewString(string(e.Value)))
		arr.SetArrayItem(len(arr.GetArray()), obj)
	}
	return string(arr.MarshalTo(nil))
}

// KvWatch subscribes to changes under prefix. cb receives a JSON array of
// {"key":..., "value":...} objects for the current snapshot and after each
// commit. The subscription runs until KvStopWatches or KvClose.
func KvWatch(prefix string, cb func(snapshotJSON string)) error {
	store, ctx, err := kvUse()
	if err != nil {
		return err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	kvWatchMtx.Lock()
	kvWatchCancels = append(kvWatchCancels, cancel)
	kvWatchMtx.Unlock()

	go func() {
		_, _ = store.Watch(watchCtx, prefix, func(entries []kvtx.WatchEntry) {
			cb(kvSnapshotJSON(entries))
		})
	}()
	return nil
}

// KvStopWatches cancels every active watch subscription. The store remains
// usable and new watches may be started afterwards.
func KvStopWatches() {
	kvWatchMtx.Lock()
	cancels := kvWatchCancels
	kvWatchCancels = nil
	kvWatchMtx.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// KvOpenHosted connects to a hosted world over WebSocket SRPC and wraps
// the remote key/value store with the same sugar surface as the embedded
// world: the Kv* functions operate against the authoritative hosted
// state, and watch subscriptions receive snapshots for commits made by
// any participant.
func KvOpenHosted(ctx context.Context, url string) error {
	conn, _, err := ws.Dial(ctx, url, nil)
	if err != nil {
		return err
	}
	mconn, err := srpc.NewWebSocketConn(ctx, conn, false, nil)
	if err != nil {
		_ = conn.CloseNow()
		return err
	}
	client := srpc.NewClientWithMuxedConn(mconn)
	store := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(client))

	kvMtx.Lock()
	defer kvMtx.Unlock()
	if kvStore != nil {
		return nil
	}
	kvStore, err = s4wave_kv_world.OpenRemote(ctx, nil, store)
	if err != nil {
		return err
	}
	kvCtx = ctx
	return nil
}

// KvIsOpen reports whether the KV API currently has an open store.
func KvIsOpen() bool {
	kvMtx.Lock()
	defer kvMtx.Unlock()
	return kvStore != nil
}
