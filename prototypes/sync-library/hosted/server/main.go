//go:build !js

// Command server hosts an authoritative world and serves its key/value
// store over WebSocket SRPC for the hosted-transport prototype.
//
// Usage: go run ./prototypes/sync-library/hosted/server [-addr :8900]
//
// Three seconds after startup the server writes server/hello so a
// subscribed client can observe a cross-process mutation.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_server "github.com/s4wave/spacewave/db/kvtx/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/prototypes/sync-library/lean"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	"github.com/sirupsen/logrus"
)

func main() {
	addr := flag.String("addr", ":8900", "http listen address")
	flag.Parse()

	ctx := context.Background()
	w, err := lean.OpenWorld(ctx)
	if err != nil {
		log.Fatalf("open world: %v", err)
	}
	defer w.Close()

	const objectKey = "sync/kv"
	if _, exists, _ := w.WS.GetObject(ctx, objectKey); !exists {
		if _, _, err := world.CreateWorldObject(ctx, w.WS, objectKey, func(bcs *block.Cursor) error {
			bcs.SetBlock(kvtx_block.NewKeyValueStoreForWorkload(kvtx_block.WorkloadClassDefault), true)
			return nil
		}); err != nil {
			log.Fatalf("create object: %v", err)
		}
	}
	if err := world_types.SetObjectType(ctx, w.WS, objectKey, s4wave_kv_world.KvStoreTypeID); err != nil {
		log.Fatalf("set object type: %v", err)
	}

	obj, err := world.MustGetObject(ctx, w.WS, objectKey)
	if err != nil {
		log.Fatalf("open object: %v", err)
	}
	var store *s4wave_kv_world.WorldBackedStore
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		var err error
		store, err = s4wave_kv_world.NewWorldBackedStore(ctx, logrus.NewEntry(logrus.New()), root.Clone(), w.WS, objectKey)
		return err
	}); err != nil {
		log.Fatalf("open backing store: %v", err)
	}
	defer store.Close()

	mux := srpc.NewMux()
	if err := kvtx_rpc.SRPCRegisterKvtx(mux, kvtx_rpc_server.NewStore(store)); err != nil {
		log.Fatalf("register kvtx: %v", err)
	}
	httpSrv, err := srpc.NewHTTPServer(mux, "/ws", nil)
	if err != nil {
		log.Fatalf("build http server: %v", err)
	}

	// Write a key shortly after startup so subscribed clients observe a
	// server-originated mutation.
	go func() {
		time.Sleep(3 * time.Second)
		tx, txErr := store.NewTransaction(ctx, true)
		if txErr != nil {
			log.Printf("server write tx: %v", txErr)
			return
		}
		if err := tx.Set(ctx, []byte("server/hello"), []byte("written-by-server")); err != nil {
			tx.Discard()
			log.Printf("server write set: %v", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Discard()
			log.Printf("server write commit: %v", err)
			return
		}
		tx.Discard()
		log.Printf("server wrote server/hello")
	}()

	log.Printf("hosted world listening on %s/ws", *addr)
	srv := &http.Server{Addr: *addr, Handler: httpSrv}
	log.Fatal(srv.ListenAndServe())
}
