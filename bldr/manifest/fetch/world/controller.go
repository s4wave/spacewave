package manifest_fetch_world

import (
	"context"
	"regexp"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/promise"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/manifest/fetch/world"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// Controller fetches Manifests via world lookups.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// fetchManifestIdRe is the parsed regex to filter manifest by.
	// if nil, accepts any
	fetchManifestIdRe *regexp.Regexp

	// mtx guards the collection lifecycle and snapshot.
	mtx sync.Mutex
	// collectionCtx ends collection work when Close releases the controller.
	collectionCtx context.Context
	// collectionCancel ends collectionCtx.
	collectionCancel context.CancelFunc
	// collectionClosed prevents new collection work after Close.
	collectionClosed bool
	// collection is the current manifest graph traversal, if any.
	collection *manifestCollection
	// collectionSnapshot is the successful traversal for its world sequence.
	collectionSnapshot *manifestCollectionSnapshot
}

type manifestCollectionKey struct {
	// seqno is the world sequence at which this collection started.
	seqno uint64
	// objectKeys is the sorted, duplicate-free collection root set.
	objectKeys []string
}

type manifestCollection struct {
	// key identifies the world collection.
	key manifestCollectionKey
	// promise resolves after the graph traversal completes.
	promise *promise.Promise[*manifestCollectionSnapshot]
}

type manifestCollectionSnapshot struct {
	// key identifies the world collection that produced this snapshot.
	key manifestCollectionKey
	// manifests is immutable after collection completes.
	manifests map[string][]*bldr_manifest_world.CollectedManifest
	// manifestErrs records non-fatal unreadable manifests from collection.
	manifestErrs []error
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: checked in Validate()
	manifestIdRe, _ := conf.ParseFetchManifestIdRe()
	collectionCtx, collectionCancel := context.WithCancel(context.Background())
	return &Controller{
		le:                le,
		bus:               bus,
		conf:              conf,
		fetchManifestIdRe: manifestIdRe,
		collectionCtx:     collectionCtx,
		collectionCancel:  collectionCancel,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"fetches manifests via world",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(ctx, inst, d))
	}
	return nil, nil
}

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(
	ctx context.Context,
	di directive.Instance,
	dir manifest.FetchManifest,
) (directive.Resolver, error) {
	if c.fetchManifestIdRe != nil && dir.GetManifestId() != "" {
		if !c.fetchManifestIdRe.MatchString(dir.GetManifestId()) {
			return nil, nil
		}
	}

	return &fetchManifestResolver{c: c, dir: dir}, nil
}

// collectManifests returns the immutable manifest map for the current world
// sequence and configured object-key set. One traversal serves all concurrent
// FetchManifest resolvers for that collection.
func (c *Controller) collectManifests(
	ctx context.Context,
	ws world.WorldState,
) (*manifestCollectionSnapshot, error) {
	for {
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return nil, err
		}
		key := manifestCollectionKey{
			seqno:      seqno,
			objectKeys: slices.Clone(c.conf.GetObjectKeys()),
		}
		slices.Sort(key.objectKeys)
		key.objectKeys = slices.Compact(key.objectKeys)

		var snapshot *manifestCollectionSnapshot
		c.mtx.Lock()
		if c.collectionClosed {
			c.mtx.Unlock()
			return nil, context.Canceled
		}
		if cached := c.collectionSnapshot; cached != nil && cached.key.equal(key) {
			snapshot = cached
			c.mtx.Unlock()
		} else {
			c.collectionSnapshot = nil
			collection := c.collection
			if collection == nil || !collection.key.equal(key) {
				collection = &manifestCollection{
					key:     key,
					promise: promise.NewPromise[*manifestCollectionSnapshot](),
				}
				c.collection = collection
				go c.collectManifestsAsync(c.collectionCtx, ws, collection)
			}
			c.mtx.Unlock()

			snapshot, err = collection.promise.Await(ctx)
			if err != nil {
				return nil, err
			}
		}

		// Confirm the traversal did not span a world revision. A sequence
		// advance re-enters the loop with a new key, where one caller starts the
		// replacement traversal and concurrent callers await the same promise.
		seqno, err = ws.GetSeqno(ctx)
		if err != nil {
			return nil, err
		}
		if snapshot.key.seqno == seqno {
			return snapshot, nil
		}
	}
}

// collectManifestsAsync collects one manifest graph and publishes only its
// successful result. The controller context, rather than any one waiter,
// controls the traversal lifetime.
func (c *Controller) collectManifestsAsync(
	ctx context.Context,
	ws world.WorldState,
	collection *manifestCollection,
) {
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsResettingUnsupportedHash(
		ctx,
		c.le,
		ws,
		nil,
		collection.key.objectKeys...,
	)
	if err == nil && ctx.Err() != nil {
		err = context.Canceled
	}
	snapshot := &manifestCollectionSnapshot{
		key:          collection.key,
		manifests:    manifests,
		manifestErrs: manifestErrs,
	}

	c.mtx.Lock()
	if c.collection == collection {
		c.collection = nil
		if err == nil && !c.collectionClosed {
			c.collectionSnapshot = snapshot
		}
	}
	c.mtx.Unlock()

	collection.promise.SetResult(snapshot, err)
}

func (k manifestCollectionKey) equal(other manifestCollectionKey) bool {
	return k.seqno == other.seqno && slices.Equal(k.objectKeys, other.objectKeys)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.mtx.Lock()
	if !c.collectionClosed {
		c.collectionClosed = true
		c.collection = nil
		c.collectionSnapshot = nil
		c.collectionCancel()
	}
	c.mtx.Unlock()
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
