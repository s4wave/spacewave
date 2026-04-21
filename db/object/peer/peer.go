package object_peer

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/scrub"
	"github.com/blang/semver/v4"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
)

// ControllerID is the controller id.
const ControllerID = "object/peer"

// Version is the component version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "stores a peer private key in an object store"

// Controller is the root resource controller.
type Controller struct {
	*bus.BusController[*Config]
	peerCtr *ccontainer.CContainer[peer.Peer]
	peerRc  *refcount.RefCount[peer.Peer]
	// xfrm transforms the stored value, may be nil if empty
	xfrm *block_transform.Transformer
}

// NewController constructs a new Controller.
func NewController(base *bus.BusController[*Config]) (*Controller, error) {
	c := &Controller{BusController: base}
	c.peerCtr = ccontainer.NewCContainer[peer.Peer](nil)
	c.peerRc = refcount.NewRefCount(nil, true, c.peerCtr, nil, c.resolvePeer)

	xfrmConf := base.GetConfig().GetTransformConf()
	if !xfrmConf.GetEmpty() {
		sfs := block_transform.NewStepFactorySet()
		sfs.AddStepFactory(transform_blockenc.NewStepFactory())
		blockXfrm, err := block_transform.NewTransformer(
			controller.ConstructOpts{Logger: base.GetLogger()},
			sfs,
			xfrmConf,
		)
		if err != nil {
			return nil, err
		}
		c.xfrm = blockXfrm
	}

	return c, nil
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		NewController,
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	c.peerRc.SetContext(ctx)
	return nil
}

// ResolvePeer adds a reference to the Peer and waits for a value.
// Returns the value, reference, and any error.
// If err != nil, value and reference will be nil.
// Returns a release function.
func (c *Controller) ResolvePeer(ctx context.Context, released func()) (peer.Peer, func(), error) {
	return c.peerRc.ResolveWithReleased(ctx, released)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case peer.GetPeer:
		// Determine peer id constraint
		if peerIDConstraint := d.GetPeerIDConstraint(); peerIDConstraint != "" {
			// Check if the peer was already resolved.
			currPeer := c.peerCtr.GetValue()
			if currPeer != nil {
				currPeerID := currPeer.GetPeerID()
				if currPeerID.String() != peerIDConstraint.String() {
					// Mismatch, ignore.
					return nil, nil
				}
			}

			// Otherwise we need to resolve the Peer to check, so return a resolver.
			return directive.R(directive.NewAccessResolver(func(ctx context.Context, released func()) (peer.GetPeerValue, func(), error) {
				rPeer, relPeer, err := c.ResolvePeer(ctx, released)
				if err != nil {
					return nil, nil, err
				}
				if rPeer == nil {
					relPeer()
					return nil, nil, nil
				}

				rPeerID := rPeer.GetPeerID()
				if rPeerID.String() != peerIDConstraint.String() {
					// Mismatch, ignore
					relPeer()
					return nil, nil, nil
				}

				// matched, return.
				return rPeer, relPeer, nil
			}), nil)
		}

		// Resolve the peer.
		return directive.R(directive.NewRefCountResolver(c.peerRc), nil)
	}

	return nil, nil
}

// resolvePeer resolves the peer.Peer accessing the object store and reading/writing the private key.
func (c *Controller) resolvePeer(ctx context.Context, released func()) (peer.Peer, func(), error) {
	objStoreVal, _, objStoreRef, err := volume.ExBuildObjectStoreAPI(ctx, c.GetBus(), false, c.GetConfig().GetObjectStoreId(), c.GetConfig().GetVolumeId(), nil)
	if err != nil {
		return nil, nil, err
	}
	defer objStoreRef.Release()

	objStore := objStoreVal.GetObjectStore()

	ktx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	defer ktx.Discard()

	key := []byte(c.GetConfig().GetObjectStoreKey())
	if len(key) == 0 {
		key = []byte("priv")
	}
	data, found, err := ktx.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	storedValue := &StoredValue{}
	if found {
		defer scrub.Scrub(data)

		// decode data if needed
		if c.xfrm != nil {
			data, err = c.xfrm.DecodeBlock(data)
			if err != nil {
				return nil, nil, err
			}
			defer scrub.Scrub(data)
		}

		// parse the message
		err := storedValue.UnmarshalVT(data)
		if err != nil {
			return nil, nil, err
		}

		// unmarshal the private key
		privKey, err := keypem.ParsePrivKeyPem([]byte(storedValue.GetPrivKeyPem()))
		if err != nil {
			return nil, nil, err
		}
		if privKey == nil {
			return nil, nil, errors.New("found value in object store but did not contain private key")
		}

		// create the Peer
		p, err := peer.NewPeer(privKey)
		if err != nil {
			return nil, nil, err
		}

		// return the peer
		c.GetLogger().
			WithField("object-store", c.GetConfig().GetObjectStoreId()).
			WithField("peer-id", p.GetPeerID().String()).
			Debug("loaded peer from object store")
		return p, nil, nil
	}

	// create a new peer
	p, err := peer.NewPeer(nil)
	if err != nil {
		return nil, nil, err
	}

	privKey, err := p.GetPrivKey(ctx)
	if err != nil {
		return nil, nil, err
	}

	// marshal the private key
	privKeyPem, err := keypem.MarshalPrivKeyPem(privKey)
	if err != nil {
		return nil, nil, err
	}

	// marshal the StoredValue
	storedValue.PrivKeyPem = string(privKeyPem)
	data, err = storedValue.MarshalVT()
	if err != nil {
		return nil, nil, err
	}
	defer scrub.Scrub(data)

	// transform if needed
	if c.xfrm != nil {
		data, err = c.xfrm.EncodeBlock(data)
		if err != nil {
			return nil, nil, err
		}
		defer scrub.Scrub(data)
	}

	// store
	if err := ktx.Set(ctx, key, data); err != nil {
		return nil, nil, err
	}

	// commit
	if err := ktx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	c.GetLogger().
		WithField("object-store", c.GetConfig().GetObjectStoreId()).
		WithField("peer-id", p.GetPeerID().String()).
		Debug("generated and stored peer in object store")

	// return peer
	return p, nil, nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
