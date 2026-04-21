package cli

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	cbus_cli "github.com/aperturerobotics/controllerbus/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/net/crypto"
	bifrost_api "github.com/s4wave/spacewave/net/daemon/api"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	pubsub_api "github.com/s4wave/spacewave/net/pubsub/api"
	stream_api_accept "github.com/s4wave/spacewave/net/stream/api/accept"
	stream_api_dial "github.com/s4wave/spacewave/net/stream/api/dial"
	stream_forwarding "github.com/s4wave/spacewave/net/stream/forwarding"
	stream_listening "github.com/s4wave/spacewave/net/stream/listening"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ClientArgs contains the client arguments and functions.
type ClientArgs struct {
	// DialConf is the dialing configuration.
	DialConf stream_api_dial.Config
	// ForwardingConf is the forwarding configuration.
	ForwardingConf stream_forwarding.Config
	// IdentifyConf is the identify configuration
	IdentifyConf peer_controller.Config
	// SubscribeConf is the configuration for the publish or subscribe call.
	SubscribeConf pubsub_api.SubscribeRequest
	// AcceptConf is the accept configuration.
	AcceptConf stream_api_accept.Config
	// ListeningConf is the listening configuration.
	ListeningConf stream_listening.Config
	// CbusConf is the controller-bus configuration.
	CbusConf cbus_cli.ClientArgs

	// ctx is the context
	ctx context.Context
	// client is the client instance
	client bifrost_api.BifrostAPIClient

	// DialAddr is the address to dial.
	DialAddr string

	// IdentifyKeyPath is the path to the key to read.
	IdentifyKeyPath string
	// IdentifyGenKey indicates we should generate the key if it doesn't exist.
	IdentifyGenKey bool

	// RemotePeerIdsCsv are the set of remote peer IDs to connect to.
	RemotePeerIdsCsv string
}

// ParseRemotePeerIdsCsv parses the RemotePeerIdsCsv field.
func (a *ClientArgs) ParseRemotePeerIdsCsv() []string {
	pts := strings.Split(a.RemotePeerIdsCsv, ",")
	var peerIds []string
	for _, pt := range pts {
		pt = strings.TrimSpace(pt)
		peerIds = append(peerIds, pt)
	}
	return peerIds
}

// BuildFlags attaches the flags to a flag set.
func (a *ClientArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "dial-addr",
			Usage:       "address to dial API on",
			Destination: &a.DialAddr,
			Value:       "127.0.0.1:5110",
		},
	}
}

// SetClient sets the client instance.
func (a *ClientArgs) SetClient(client bifrost_api.BifrostAPIClient) {
	a.client = client
}

// BuildClient builds the client or returns it if it has been set.
func (a *ClientArgs) BuildClient() (bifrost_api.BifrostAPIClient, error) {
	if a.client != nil {
		return a.client, nil
	}

	if a.DialAddr == "" {
		return nil, errors.New("dial address is not set")
	}

	nconn, err := net.Dial("tcp", a.DialAddr)
	if err != nil {
		return nil, err
	}

	muxedConn, err := srpc.NewMuxedConn(nconn, true, nil)
	if err != nil {
		return nil, err
	}
	conn := srpc.NewClientWithMuxedConn(muxedConn)
	a.client = bifrost_api.NewBifrostAPIClient(conn)
	return a.client, nil
}

// BuildBifrostCommand returns the controller-bus sub-command set.
func (a *ClientArgs) BuildBifrostCommand() *cli.Command {
	bifrostCmds := a.BuildCommands()
	return &cli.Command{
		Name:        "bifrost",
		Usage:       "Bifrost network-router sub-commands.",
		Subcommands: bifrostCmds,
	}
}

// BuildCommands attaches the commands.
func (a *ClientArgs) BuildCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:   "local-peers",
			Usage:  "returns local peer info",
			Action: a.RunPeerInfo,
		},
		{
			Name:   "identify",
			Usage:  "Private key will be loaded with a peer controller",
			Action: a.RunIdentifyController,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "peer-priv",
					Usage:       "path to private key file",
					Destination: &a.IdentifyKeyPath,
				},
				&cli.BoolFlag{
					Name:        "generate-priv",
					Usage:       "if set, generate private key if file does not exist",
					Destination: &a.IdentifyGenKey,
				},
			},
		},
		{
			Name:   "subscribe",
			Usage:  "Subscribe to a pubsub channel with a private key or mounted peer and publish base64 w/ newline delim from stdin.",
			Action: a.RunSubscribe,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "peer-priv",
					Usage:       "path to private key file, specify this or peer-id",
					Destination: &a.IdentifyKeyPath,
				},
				&cli.BoolFlag{
					Name:        "generate-priv",
					Usage:       "if set with peer-priv, generate private key if file does not exist",
					Destination: &a.IdentifyGenKey,
				},
				&cli.StringFlag{
					Name:        "peer-id",
					Usage:       "peer identifier to lookup and use, specify this or peer-priv",
					Destination: &a.SubscribeConf.PeerId,
				},
				&cli.StringFlag{
					Name:        "channel-id",
					Usage:       "channel id",
					Destination: &a.SubscribeConf.ChannelId,
				},
			},
		},
		{
			Name:   "forward",
			Usage:  "Protocol ID will be forwarded to the target multiaddress",
			Action: a.RunForwarding,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "peer-id",
					Usage:       "peer ID to match incoming streams to",
					Destination: &a.ForwardingConf.PeerId,
				},
				&cli.StringFlag{
					Name:        "protocol-id",
					Usage:       "protocol ID to match incoming streams to",
					Destination: &a.ForwardingConf.ProtocolId,
				},
				&cli.StringFlag{
					Name:        "target",
					Usage:       "target multiaddr to forward streams to",
					Destination: &a.ForwardingConf.TargetMultiaddr,
				},
			},
		},
		{
			Name:   "accept",
			Usage:  "Single incoming stream with Protocol ID will be accepted",
			Action: a.RunAccept,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "local-peer-id",
					Usage:       "local peer ID to match incoming streams to",
					Destination: &a.AcceptConf.LocalPeerId,
				},
				&cli.StringFlag{
					Name:        "protocol-id",
					Usage:       "protocol ID to match incoming streams to",
					Destination: &a.AcceptConf.ProtocolId,
				},
				&cli.StringFlag{
					Name:        "remote-peer-ids",
					Usage:       "remote peer ids, comma separated, to match, if empty accepts any",
					Destination: &a.RemotePeerIdsCsv,
				},
				&cli.Uint64Flag{
					Name:        "transport-id",
					Usage:       "if set, filter the transport id",
					Destination: &a.AcceptConf.TransportId,
				},
			},
		},
		{
			Name:   "dial",
			Usage:  "Single outgoing stream with Protocol ID will be dialed",
			Action: a.RunDial,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "local-peer-id",
					Usage:       "local peer ID to dial from, can be empty",
					Destination: &a.DialConf.LocalPeerId,
				},
				&cli.StringFlag{
					Name:        "protocol-id",
					Usage:       "protocol ID to dial with",
					Destination: &a.DialConf.ProtocolId,
				},
				&cli.StringFlag{
					Name:        "peer-id",
					Usage:       "remote peer id to dial",
					Destination: &a.DialConf.PeerId,
				},
				&cli.Uint64Flag{
					Name:        "transport-id",
					Usage:       "if set, filter the transport id",
					Destination: &a.DialConf.TransportId,
				},
			},
		},
		{
			Name:   "listen",
			Usage:  "Listen on the multiaddress and forward the connection to a remote stream.",
			Action: a.RunListen,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "peer-id",
					Usage:       "peer ID to route traffic to",
					Destination: &a.ListeningConf.RemotePeerId,
				},
				&cli.StringFlag{
					Name:        "from-peer-id",
					Usage:       "peer ID to route traffic from, optional",
					Destination: &a.ListeningConf.LocalPeerId,
				},
				&cli.StringFlag{
					Name:        "protocol-id",
					Usage:       "protocol ID for outgoing streams",
					Destination: &a.ListeningConf.ProtocolId,
				},
				&cli.StringFlag{
					Name:        "listen",
					Usage:       "listen multiaddr",
					Destination: &a.ListeningConf.ListenMultiaddr,
				},
			},
		},
	}
}

// SetContext sets the context.
func (a *ClientArgs) SetContext(c context.Context) {
	a.ctx = c
}

// GetContext returns the context.
func (a *ClientArgs) GetContext() context.Context {
	if c := a.ctx; c != nil {
		return c
	}
	return context.TODO()
}

// LoadOrGenerateIdentifyKey loads or generates the IdentifyKeyPath.
func (a *ClientArgs) LoadOrGenerateIdentifyKey() ([]byte, crypto.PrivKey, error) {
	if a.IdentifyKeyPath == "" {
		return nil, nil, errors.New("identification private key path not set")
	}

	var privKey crypto.PrivKey
	var dat []byte
	var err error
	if a.IdentifyGenKey {
		if _, err := os.Stat(a.IdentifyKeyPath); os.IsNotExist(err) {
			npeer, err := peer.NewPeer(nil)
			if err != nil {
				return nil, nil, err
			}
			privKey, err := npeer.GetPrivKey(a.ctx)
			if err != nil {
				return nil, nil, err
			}
			dat, err = confparse.MarshalPrivateKeyPEM(privKey)
			if err != nil {
				return nil, nil, err
			}
			if err := os.WriteFile(a.IdentifyKeyPath, dat, 0o600); err != nil {
				return nil, nil, err
			}
		}
	}

	if len(dat) == 0 {
		dat, err = os.ReadFile(a.IdentifyKeyPath)
		if err != nil {
			return nil, nil, err
		}
		privKey = nil
	}

	if privKey == nil {
		privKey, err = confparse.ParsePrivateKey(string(dat))
		if err != nil {
			return nil, nil, err
		}
	}

	return dat, privKey, nil
}
