package tptaddr_static

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/tptaddr"
)

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/tptaddr/static"

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// Controller implements the static tptaddr address list controller.
// Resolves LookupTptAddr directives with a static list of addresses.
type Controller struct {
	// peers is the map between peer ids and addresses
	peers map[string][]tptaddr.LookupTptAddrValue
}

// ParsePeerAddressMap parses the list of addresses to a peers map.
//
// Returns a list of errors for skipped addresses.
func ParsePeerAddressMap(addressesWithPeerIDs []string) (map[string][]string, []error) {
	// Prepare the peer address index before validating each configured address.
	var errs []error
	peers := make(map[string][]string)
	for _, addr := range addressesWithPeerIDs {
		peerIDStr, tptaddr, found := strings.Cut(addr, "|")
		tptaddr = strings.TrimSpace(tptaddr)
		if !found || !strings.Contains(tptaddr, "|") {
			errs = append(errs, errors.New("invalid peer address: format is {peer-id}|{transport-type}|{address}"))
			continue
		}
		pid, err := peer.IDB58Decode(strings.TrimSpace(peerIDStr))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		pidString := pid.String()
		peers[pidString] = append(peers[pidString], tptaddr)
	}

	// Sort and deduplicate each peer's transport addresses.
	for k, sl := range peers {
		slices.Sort(sl)
		sl = slices.Compact(sl)
		peers[k] = sl
	}
	return peers, errs
}

// NewController constructs a new controller.
func NewController(conf *Config) (*Controller, error) {
	// Parse configured addresses and reject the first invalid entry.
	peers, errs := ParsePeerAddressMap(conf.GetAddresses())
	if len(errs) != 0 {
		return nil, errs[0]
	}

	return &Controller{
		peers: peers,
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"static list of peer transport addresses",
	)
}

// Execute runs the controller; the static address map needs no execution loop.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	if d, ok := dir.(tptaddr.LookupTptAddr); ok {
		return c.resolveLookupTptAddr(ctx, di, d)
	}

	return nil, nil
}

// resolveLookupTptAddr resolves a LookupTptAddr directive.
func (c *Controller) resolveLookupTptAddr(
	ctx context.Context,
	di directive.Instance,
	dir tptaddr.LookupTptAddr,
) ([]directive.Resolver, error) {
	targetPeerID := dir.LookupTptAddrTargetPeerId()
	targetPeerIDString := targetPeerID.String()
	addrs := c.peers[targetPeerIDString]
	if len(addrs) != 0 {
		return []directive.Resolver{directive.NewValueResolver(addrs)}, nil
	}
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
