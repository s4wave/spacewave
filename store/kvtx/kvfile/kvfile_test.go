package store_kvtx_kvfile

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/go-kvfile"
	kvtx_kvfile "github.com/aperturerobotics/hydra/kvtx/kvfile"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	store_kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/sirupsen/logrus"
)

// TestKvfile tests the kvfile volume on top of inmem.
func TestKvfile(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	// build an in-memory store first to commit to the kvfile.
	writeStore := store_kvtx_inmem.NewStore()
	writeKtx := store_kvtx.NewKVTx(
		ctx,
		"test/kvfile/inmem",
		kvkey,
		store_kvtx_vlogger.NewVLogger(le, writeStore),
		nil,
	).(*store_kvtx.KVTx)
	writeKtxCtx, writeKtxCancel := context.WithCancel(ctx)
	go func() {
		_ = writeKtx.Execute(writeKtxCtx)
	}()

	testPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	testPeerPriv, err := testPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := writeKtx.StorePeerPriv(ctx, testPeerPriv); err != nil {
		t.Fatal(err.Error())
	}

	// convert it to a kvfile
	var buf bytes.Buffer
	if err := kvtx_kvfile.KvfileFromStore(ctx, &buf, writeStore); err != nil {
		t.Fatal(err.Error())
	}
	writeKtxCancel()

	bufReader := bytes.NewReader(buf.Bytes())
	rdr, err := kvfile.BuildReader(bufReader, uint64(buf.Len()))
	if err != nil {
		t.Fatal(err.Error())
	}

	ktx := store_kvtx.NewKVTx(
		ctx,
		"test/kvfile",
		kvkey,
		store_kvtx_vlogger.NewVLogger(le, NewStore(rdr)),
		&store_kvtx.Config{},
	).(*store_kvtx.KVTx)
	/*
		if err := store_test.TestAll(ctx, ktx); err != nil {
			t.Fatal(err.Error())
		}*/
	_, err = ktx.LoadPeerPriv(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
}
