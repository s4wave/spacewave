//go:build !js && !wasip1

package main

import (
	"context"
	"os"
	"strings"

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

// archiveArgs configures reading from archive-peers. Each archive dials this
// process at its own address, so a run logs this peer's id and each address
// for archive-peer's --peer.
type archiveArgs struct {
	// key is this peer's PEM private key.
	key string
	// archives are the archives, as peer id@UDP host:port to listen on for
	// that archive's dial, in the order a copy asks them.
	archives cli.StringSlice
	// bucketID is the block store bucket the archives serve.
	bucketID string
	// protocolContext is the block exchange context the archives use.
	protocolContext string
}

// flags binds the archives' flags.
func (a *archiveArgs) flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "archive", Usage: "archive-peer to ask, as peer-id@host:port to listen on for its dial; repeat for each, in the order to ask them", Destination: &a.archives},
		&cli.StringFlag{Name: "archive-key", Usage: "PEM private key of this peer", Destination: &a.key},
		&cli.StringFlag{Name: "archive-bucket-id", Usage: "block store bucket the archives serve", Destination: &a.bucketID},
		&cli.StringFlag{Name: "archive-protocol-context", Usage: "block exchange protocol context, as given to archive-peer", Destination: &a.protocolContext},
	}
}

// open starts a peer for each archive and returns sources that ask them, in
// order, with the function that stops them.
func (a *archiveArgs) open(ctx context.Context, le *logrus.Entry) ([]packcopy.Source, func(), error) {
	// Collect the sources and what stops each.
	var sources []packcopy.Source
	var releases []func()
	release := func() {
		for _, r := range releases {
			r()
		}
	}

	// Start a peer for each archive, in order.
	for _, spec := range a.archives.Value() {
		id, listen, ok := strings.Cut(spec, "@")
		if !ok {
			release()
			return nil, nil, errors.Errorf("--archive %q must be peer-id@host:port", spec)
		}
		src, rel, err := a.openOne(ctx, le, id, listen)
		if err != nil {
			release()
			return nil, nil, err
		}
		sources = append(sources, src)
		releases = append(releases, rel)
	}
	return sources, release, nil
}

// openOne starts a peer that listens on listen for the archive id and returns
// a source that asks it.
func (a *archiveArgs) openOne(ctx context.Context, le *logrus.Entry, id, listen string) (packcopy.Source, func(), error) {
	// Parse the archive's id and this peer's key.
	archiveID, err := peer.IDB58Decode(id)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse --archive peer id")
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
		ListenAddr: listen,
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
