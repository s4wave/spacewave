package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Args configures the archive peer.
type Args struct {
	// File is the stopped s4db file to serve.
	File string
	// Key is the PEM private key of this peer.
	Key string
	// GenKey creates Key, prints its peer id, and exits.
	GenKey bool
	// Listen is the UDP host:port to listen on.
	Listen string
	// Peer is the one peer served, as peer@host:port.
	Peer string
	// BucketID is the block store bucket the peer asks for.
	BucketID string
	// ProtocolContext is the block exchange protocol context the peer uses,
	// such as its SharedObject id.
	ProtocolContext string
}

// BuildFlags binds the archive peer's flags.
func (a *Args) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "file", Usage: "stopped s4db file to serve", Destination: &a.File},
		&cli.StringFlag{Name: "key", Usage: "PEM private key of this peer", Destination: &a.Key, Required: true},
		&cli.BoolFlag{Name: "gen-key", Usage: "create --key, print its peer id, and exit", Destination: &a.GenKey},
		&cli.StringFlag{Name: "listen", Usage: "UDP host:port to listen on", Value: ":4102", Destination: &a.Listen},
		&cli.StringFlag{Name: "peer", Usage: "the one peer served, as peer@host:port", Destination: &a.Peer},
		&cli.StringFlag{Name: "bucket-id", Usage: "block store bucket the peer asks for", Destination: &a.BucketID},
		&cli.StringFlag{Name: "protocol-context", Usage: "block exchange protocol context, such as the SharedObject id", Destination: &a.ProtocolContext},
	}
}

// Run creates the key, or serves the archive until interrupted.
func (a *Args) Run(*cli.Context) error {
	// Create the key and report the peer id to name in the peer's settings.
	if a.GenKey {
		id, err := createKey(a.Key)
		if err != nil {
			return err
		}
		_, err = os.Stdout.WriteString(id.String() + "\n")
		return err
	}
	if a.File == "" || a.Peer == "" || a.BucketID == "" || a.ProtocolContext == "" {
		return errors.New("--file, --peer, --bucket-id and --protocol-context are required")
	}

	// Serve until interrupted.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err := a.serve(ctx, logrus.NewEntry(logrus.StandardLogger()))
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// serve serves the archive until ctx ends.
func (a *Args) serve(ctx context.Context, le *logrus.Entry) error {
	// Parse the one peer served and this peer's key.
	remoteID, remoteAddr, ok := strings.Cut(a.Peer, "@")
	if !ok {
		return errors.New("--peer must be peer@host:port")
	}
	served, err := peer.IDB58Decode(remoteID)
	if err != nil {
		return errors.Wrap(err, "parse --peer")
	}
	pemData, err := os.ReadFile(a.Key)
	if err != nil {
		return err
	}
	key, err := keypem.ParsePrivKeyPem(pemData)
	if err != nil {
		return errors.Wrap(err, "parse --key")
	}

	// Open only the existing file, read-only, and refuse one another process holds.
	if _, err := os.Stat(a.File); err != nil {
		return err
	}
	db, err := s4db.Open(a.File, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer db.Close()
	shared, err := db.Shared()
	if err != nil {
		return err
	}
	if shared {
		return errors.New("refusing a file open in another process")
	}

	// Answer the bucket lookup for the served bucket from the file.
	parent, _, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	defer parent.Close()
	arch := &archive{le: le, db: db, keys: store_kvkey.NewDefaultKVKey(), bucketID: a.BucketID}
	if _, err := parent.AddController(ctx, arch, nil); err != nil {
		return err
	}

	// Run the transport, admitting only the served peer and its block reads.
	admit := func(_ context.Context, id peer.ID) error {
		if id != served {
			return errors.Errorf("peer %s is not served", id)
		}
		return nil
	}
	st, err := transport.NewSessionTransport(le, parent, key, "", "",
		transport.WithPeerAuthorizer(admit),
		transport.WithDEXRequestAuthorizer(func(ctx context.Context, _ *dex_solicit.Config, id peer.ID) (dex_solicit.RequestAccess, error) {
			if err := admit(ctx, id); err != nil {
				return nil, err
			}
			return staticAccess{}, nil
		}),
	)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- st.Execute(ctx) }()
	if err := st.AwaitReady(ctx); err != nil {
		return err
	}
	child := st.GetChildBus()

	// Serve the bucket over the block exchange.
	_, _, dexRef, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](
		ctx,
		child,
		resolver.NewLoadControllerWithConfig(&dex_solicit.Config{
			BucketId:        a.BucketID,
			ProtocolContext: []byte(a.ProtocolContext),
		}),
		nil,
	)
	if err != nil {
		return err
	}
	defer dexRef.Release()

	// Listen, dial the served peer, and hold its link: a link this end does
	// not hold closes shortly after each dial.
	addr, err := st.StartUDP(ctx, transport.UDPConfig{
		ListenAddr: a.Listen,
		Peers:      map[peer.ID]string{served: remoteAddr},
		NamedOnly:  true,
	})
	if err != nil {
		return err
	}
	hold := directive.NewTypedCallbackHandler[link.MountedLink](
		func(directive.TypedAttachedValue[link.MountedLink]) {
			le.WithField("peer", served.String()).Info("linked")
		}, nil, nil, nil,
	)
	_, linkRef, err := child.AddDirective(ctx, link.NewEstablishLinkWithPeer(st.GetPeerID(), served), hold)
	if err != nil {
		return err
	}
	defer linkRef.Release()
	le.WithField("addr", addr.String()).
		WithField("peer", st.GetPeerID().String()).
		WithField("serves", served.String()).
		WithField("bucket-id", a.BucketID).
		Info("archive peer listening")
	return <-errCh
}

// createKey writes a new Ed25519 key to path, which must not exist, and
// returns its peer id.
func createKey(path string) (peer.ID, error) {
	// Generate the key and encode it.
	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return "", err
	}
	data, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return "", err
	}

	// Write it to a new file only its owner reads.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return peer.IDFromPrivateKey(priv)
}
