package object_peer

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func TestObjectStorePeer(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	// build block enc conf
	blockEncKey := make([]byte, 32)
	_, _ = rand.Read(blockEncKey) //nolint:gosec
	blockEncConf, err := (&transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      blockEncKey,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}

	// build controller config
	conf := &Config{
		ObjectStoreId: "test-object-store-peer",
		TransformConf: &block_transform.Config{Steps: []*block_transform.StepConfig{
			{Id: "hydra/transform/blockenc", Config: blockEncConf},
		}},
	}

	// run the controller the first time
	var createdPeerID peer.ID
	if err := func() error {
		peerCtrl, err := NewController(bus.NewBusController(le, tb.Bus, conf, ControllerID, Version, controllerDescrip))
		if err != nil {
			t.Fatal(err.Error())
		}
		relPeerCtrl, err := tb.Bus.AddController(ctx, peerCtrl, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer relPeerCtrl()

		// resolve the peer (should store the id)
		createdPeer, relCreatedPeer, err := peerCtrl.ResolvePeer(ctx, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		createdPeerID = createdPeer.GetPeerID()
		relCreatedPeer()
		return nil
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// run the controller the second time (should get the peer from storage)
	if err := func() error {
		peerCtrl, _, peerCtrlRef, err := loader.WaitExecControllerRunningTyped[*Controller](
			ctx,
			tb.Bus,
			resolver.NewLoadControllerWithConfig(conf),
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer peerCtrlRef.Release()

		// resolve the peer (should load the id)
		loadedPeer, relLoadedPeer, err := peerCtrl.ResolvePeer(ctx, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		loadedPeerID := loadedPeer.GetPeerID()
		relLoadedPeer()

		loadedPeerIDStr, createdPeerIDStr := loadedPeerID.String(), createdPeerID.String()
		if loadedPeerIDStr != createdPeerIDStr {
			return errors.Errorf("expected to load peer id %s but got %s", createdPeerIDStr, loadedPeerIDStr)
		}
		return nil
	}(); err != nil {
		t.Fatal(err.Error())
	}
}
