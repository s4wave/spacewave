package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block/packcopy"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// archiveArgs configures reading from an archive-peer. The archive dials this
// process, so a run logs its peer id and address for archive-peer's --peer.
type archiveArgs struct {
	// key is this peer's PEM private key.
	key string
	// listen is the UDP host:port to listen on.
	listen string
	// peerID is the archive's peer id.
	peerID string
	// bucketID is the block store bucket the archive serves.
	bucketID string
	// protocolContext is the block exchange context the archive uses.
	protocolContext string
}

// flags binds the archive's flags.
func (a *archiveArgs) flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "archive-peer", Usage: "archive-peer's peer id; empty reads no archive", Destination: &a.peerID},
		&cli.StringFlag{Name: "archive-key", Usage: "PEM private key of this peer", Destination: &a.key},
		&cli.StringFlag{Name: "archive-listen", Usage: "UDP host:port the archive dials", Value: ":4103", Destination: &a.listen},
		&cli.StringFlag{Name: "archive-bucket-id", Usage: "block store bucket the archive serves", Destination: &a.bucketID},
		&cli.StringFlag{Name: "archive-protocol-context", Usage: "block exchange protocol context, as given to archive-peer", Destination: &a.protocolContext},
	}
}

// enabled reports whether a run reads an archive.
func (a *archiveArgs) enabled() bool {
	return a.peerID != ""
}

// open starts this peer and returns a source that asks the archive.
func (a *archiveArgs) open(ctx context.Context, le *logrus.Entry) (packcopy.Source, func(), error) {
	// Parse the archive's id and this peer's key.
	archiveID, err := peer.IDB58Decode(a.peerID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse --archive-peer")
	}
	pemData, err := os.ReadFile(a.key)
	if err != nil {
		return nil, nil, err
	}
	key, err := keypem.ParsePrivKeyPem(pemData)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse --archive-key")
	}

	// Run the transport, admitting only the archive.
	ctx, cancel := context.WithCancel(ctx)
	parent, _, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	st, err := transport.NewSessionTransport(le, parent, key, "", "",
		transport.WithPeerAuthorizer(func(_ context.Context, id peer.ID) error {
			if id != archiveID {
				return errors.Errorf("peer %s is not the archive", id)
			}
			return nil
		}),
	)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	go func() { _ = st.Execute(ctx) }()
	if err := st.AwaitReady(ctx); err != nil {
		cancel()
		return nil, nil, err
	}

	// Listen for the archive's dial and report where.
	addr, err := st.StartUDP(ctx, transport.UDPConfig{
		ListenAddr: a.listen,
		Peers:      map[peer.ID]string{archiveID: ""},
		NamedOnly:  true,
	})
	if err != nil {
		cancel()
		return nil, nil, err
	}
	le.WithField("peer", st.GetPeerID().String()+"@"+addr.String()).Info("waiting for archive-peer")

	// Ask for blocks over the exchange.
	exchange, _, ref, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](
		ctx,
		st.GetChildBus(),
		resolver.NewLoadControllerWithConfig(&dex_solicit.Config{
			BucketId:        a.bucketID,
			ProtocolContext: []byte(a.protocolContext),
		}),
		nil,
	)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return dex_solicit.NewStore(exchange), func() {
		ref.Release()
		cancel()
	}, nil
}
