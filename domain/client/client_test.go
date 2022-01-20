package identity_domain_client

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
	stream_drpc_client "github.com/aperturerobotics/bifrost/stream/drpc/client"
	"github.com/aperturerobotics/bifrost/testbed"
	"github.com/aperturerobotics/bifrost/transport/common/dialer"
	"github.com/aperturerobotics/bifrost/transport/inproc"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/identity"
	identity_domain_server "github.com/aperturerobotics/identity/domain/server"
	identity_static "github.com/aperturerobotics/identity/domain/static"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

// TestDomainClient tests the client and server.
func TestDomainClient(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// tb1: client
	tb1, err := testbed.NewTestbed(ctx, le.WithField("testbed", 1), testbed.TestbedOpts{NoEcho: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	tb1.StaticResolver.AddFactory(NewFactory(tb1.Bus))

	// tb2: server
	tb2, err := testbed.NewTestbed(ctx, le.WithField("testbed", 2), testbed.TestbedOpts{NoEcho: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	tb2.StaticResolver.AddFactory(identity_domain_server.NewFactory(tb2.Bus))
	tb2.StaticResolver.AddFactory(identity_static.NewFactory(tb2.Bus))

	tb1PeerID, err := peer.IDFromPrivateKey(tb1.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	tb2PeerID, err := peer.IDFromPrivateKey(tb2.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// generate entity and add to tb2
	entityUUID := uuid.NewV4()
	entityID, domainID := "test-entity", "test-domain"
	ent, err := identity.EntityWithPrivKey(
		domainID, entityID,
		entityUUID.String(),
		tb2.PrivKey,
		"", nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, _, staticRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb2.Bus,
		resolver.NewLoadControllerWithConfig(
			&identity_static.Config{
				Entities: []*identity.Entity{ent},
			},
		),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer staticRef.Release()

	// tb2: Run the server
	_, _, serverRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb2.Bus,
		resolver.NewLoadControllerWithConfig(
			&identity_domain_server.Config{
				PeerIds: []string{tb2PeerID.Pretty()},
			},
		),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer serverRef.Release()

	// tb1: run the client
	_, _, clientRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb1.Bus,
		resolver.NewLoadControllerWithConfig(
			&Config{
				PeerId: tb1PeerID.Pretty(),
				ClientOpts: &stream_drpc_client.Config{
					ServerPeerIds: []string{
						tb2PeerID.Pretty(),
					},
				},
			},
		),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer clientRef.Release()

	// tb1 -> tb2 inproc
	tp2 := inproc.BuildInprocController(tb2.Logger, tb2.Bus, tb2PeerID, &inproc.Config{
		TransportPeerId: tb2PeerID.Pretty(),
	})
	tpt2dialer := &dialer.DialerOpts{
		Address: inproc.NewAddr(tb2PeerID).String(),
	}
	tp1 := inproc.BuildInprocController(tb1.Logger, tb1.Bus, tb1PeerID, &inproc.Config{
		TransportPeerId: tb1PeerID.Pretty(),
		Dialers: map[string]*dialer.DialerOpts{
			tb2PeerID.Pretty(): tpt2dialer,
		},
	})

	go tb2.Bus.ExecuteController(ctx, tp2)
	go tb1.Bus.ExecuteController(ctx, tp1)

	// connect them
	tpt2, _ := tp2.GetTransport(ctx)
	tpt1, _ := tp1.GetTransport(ctx)
	tpt1.(*inproc.Inproc).ConnectToInproc(ctx, tpt2.(*inproc.Inproc))
	tpt2.(*inproc.Inproc).ConnectToInproc(ctx, tpt1.(*inproc.Inproc))

	// run the query
	val, err := identity.ExIdentityLookupEntity(ctx, tb1.Bus, domainID, entityID)
	if val == nil && err == nil {
		err = errors.New("not found")
	}
	if err == nil {
		err = val.GetError()
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	oent := val.GetEntity()
	if val.IsNotFound() {
		t.Fatal("returned not found")
	}

	t.Logf("retrieved entity: %#v", oent)
	if err := oent.Validate(); err != nil {
		t.Fatal(err.Error())
	}
}
