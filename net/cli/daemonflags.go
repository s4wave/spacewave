package cli

import (
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	configset "github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/pkg/errors"
	link_establish_controller "github.com/s4wave/spacewave/net/link/establish"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	"github.com/s4wave/spacewave/net/transport/common/pconn"
	udptpt "github.com/s4wave/spacewave/net/transport/udp"
	wtpt "github.com/s4wave/spacewave/net/transport/websocket"
)

// DaemonArgs contains common flags for bifrost-powered daemons.
type DaemonArgs struct {
	WebsocketListen string
	UDPListen       string
	HoldOpenLinks   bool
	Pubsub          string

	// EstablishPeers is a list of peers to establish
	// peer-id comma separated
	EstablishPeers cli.StringSlice
	// UDPPeers is a static peer list
	// peer-id@address
	UDPPeers cli.StringSlice
	// WebsocketPeers is a static peer list
	// peer-id@address
	WebsocketPeers cli.StringSlice
}

// BuildFlags attaches the flags to a flag set.
func (a *DaemonArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:        "hold-open-links",
			Usage:       "if set, hold open links without an inactivity timeout",
			EnvVars:     []string{"BIFROST_HOLD_OPEN_LINKS"},
			Destination: &a.HoldOpenLinks,
		},
		&cli.StringFlag{
			Name:        "websocket-listen",
			Usage:       "if set, will listen on address for websocket connections, ex :5111",
			EnvVars:     []string{"BIFROST_WS_LISTEN"},
			Destination: &a.WebsocketListen,
		},
		&cli.StringFlag{
			Name:        "udp-listen",
			Usage:       "if set, will listen on address for udp connections, ex :5112",
			EnvVars:     []string{"BIFROST_UDP_LISTEN"},
			Destination: &a.UDPListen,
		},
		&cli.StringSliceFlag{
			Name:    "establish-peers",
			Usage:   "if set, request establish links to list of peer ids",
			EnvVars: []string{"BIFROST_ESTABLISH_PEERS"},
			Value:   &a.EstablishPeers,
		},
		&cli.StringSliceFlag{
			Name:    "udp-peers",
			Usage:   "list of peer-id@address known UDP peers",
			EnvVars: []string{"BIFROST_UDP_PEERS"},
			Value:   &a.UDPPeers,
		},
		&cli.StringSliceFlag{
			Name:    "websocket-peers",
			Usage:   "list of peer-id@address known WebSocket peers",
			EnvVars: []string{"BIFROST_WS_PEERS"},
			Value:   &a.WebsocketPeers,
		},
		&cli.StringFlag{
			Name:        "pubsub",
			Usage:       buildPubsubUsage(),
			EnvVars:     []string{"BIFROST_PUBSUB"},
			Destination: &a.Pubsub,
		},
	}
}

// ApplyFactories applies any extra factories necessary on top of the core set.
func (a *DaemonArgs) ApplyFactories(b bus.Bus, sr *static.Resolver) {
	for _, f := range pubsubFactories {
		sr.AddFactory(f(b))
	}
}

// ApplyToConfigSet applies controller configurations to a config set.
// Map is from string descriptor to config object.
func (a *DaemonArgs) ApplyToConfigSet(confSet configset.ConfigSet, overwrite bool) error {
	apply := func(id string, conf config.Config) {
		if !overwrite {
			if _, ok := confSet[id]; ok {
				return
			}
		}
		confSet[id] = configset.NewControllerConfig(1, conf)
	}
	if len(a.EstablishPeers.Value()) != 0 {
		establishConf := &link_establish_controller.Config{
			PeerIds: a.EstablishPeers.Value(),
		}
		if err := establishConf.Validate(); err != nil {
			return errors.Wrap(err, "establish-peers")
		}
		apply("establish-peers", establishConf)
	}
	if a.HoldOpenLinks {
		apply("hold-open", &link_holdopen_controller.Config{})
	}

	if a.WebsocketListen != "" {
		staticPeers, err := parseDialerAddrs(a.WebsocketPeers)
		if err != nil {
			return errors.Wrap(err, "websocket-peers")
		}

		apply("websocket", &wtpt.Config{
			Dialers:    staticPeers,
			ListenAddr: a.WebsocketListen,
		})
	}

	if a.UDPListen != "" {
		staticPeers, err := parseDialerAddrs(a.UDPPeers)
		if err != nil {
			return errors.Wrap(err, "udp-peers")
		}

		apply("udp", &udptpt.Config{
			Dialers:    staticPeers,
			ListenAddr: a.UDPListen,
			PacketOpts: &pconn.Opts{},
		})
	}

	if a.Pubsub != "" {
		conf, err := a.callPubsubProvider(strings.ToLower(a.Pubsub))
		if err != nil {
			return err
		}
		apply("pubsub", conf)
	}

	return nil
}

// callPubsubProvider calls a pubsub provider preset by id or returns an error
func (a *DaemonArgs) callPubsubProvider(id string) (config.Config, error) {
	prov, ok := pubsubProviders[id]
	if !ok {
		return nil, errors.Errorf("unknown pubsub provider: %s", id)
	}
	return prov(a)
}
