// Package device_flowgraph runs the Flowgraph nodes placed on this Device.
package device_flowgraph

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Reconciler runs the Flowgraph nodes placed on one Device. It watches the
// World for placements and the DevicePolicy for the allowed node types,
// compiles each allowed node through its node type, applies the compiled
// entries as one ConfigSet, and writes each node's state into the Device
// object. A node that fails shows as rejected with its error while the other
// nodes keep running. The daemon is the only writer of the Device object.
type Reconciler struct {
	le *logrus.Entry
	// b is the bus node types resolve on and the ConfigSet applies on.
	b bus.Bus
	// engine is the World engine of the Space holding the Device.
	engine world.Engine
	// policy supplies the allowed node types.
	policy *device_policy.PolicyStore
	// deviceKey is the object key of this daemon's Device.
	deviceKey string
	// peerID is the peer ID of this daemon's Device.
	peerID string

	// wake holds a pending reconcile. Every watched source sets it.
	wake chan struct{}
	// types holds the lookup of each node type in use.
	types *nodeTypes
	// applier applies the compiled entries.
	applier *configApplier
}

// NewReconciler constructs a Reconciler for the Device at deviceKey with the
// given peer ID.
func NewReconciler(
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	policy *device_policy.PolicyStore,
	deviceKey, peerID string,
) *Reconciler {
	r := &Reconciler{
		le:        le,
		b:         b,
		engine:    engine,
		policy:    policy,
		deviceKey: deviceKey,
		peerID:    peerID,
		wake:      make(chan struct{}, 1),
	}
	r.types = newNodeTypes(b, r.notify)
	r.applier = newConfigApplier(b, r.notify)
	return r
}

// Run reconciles until ctx ends, then releases every node's configs.
func (r *Reconciler) Run(ctx context.Context) error {
	// Release the lookups and the configs on every exit.
	defer r.types.Release()
	defer r.applier.Release()

	// Watch the World and the policy, and reconcile after each change.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error { return r.watchWorld(egCtx) })
	eg.Go(func() error { return r.watchPolicy(egCtx) })
	eg.Go(func() error {
		for {
			select {
			case <-egCtx.Done():
				return egCtx.Err()
			case <-r.wake:
			}
			if err := r.reconcile(egCtx); err != nil && egCtx.Err() == nil {
				r.le.WithError(err).Warn("failed to reconcile flowgraph nodes")
			}
		}
	})
	if err := eg.Wait(); ctx.Err() == nil {
		return err
	}
	return nil
}

// notify schedules a reconcile.
func (r *Reconciler) notify() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// watchWorld schedules a reconcile for the current World and for every change.
func (r *Reconciler) watchWorld(ctx context.Context) error {
	seqno, err := r.engine.GetSeqno(ctx)
	if err != nil {
		return err
	}
	for {
		r.notify()
		seqno, err = r.engine.WaitSeqno(ctx, seqno+1)
		if err != nil {
			return err
		}
	}
}

// watchPolicy schedules a reconcile for the current policy and for every
// change.
func (r *Reconciler) watchPolicy(ctx context.Context) error {
	var last *device_policy.DevicePolicy
	for {
		policy, err := r.policy.WaitChange(ctx, last)
		if err != nil {
			return err
		}
		last = policy
		r.notify()
	}
}

// reconcile brings the running nodes and the Device object to the placed nodes
// the policy allows.
func (r *Reconciler) reconcile(ctx context.Context) error {
	// Read the nodes placed on this Device.
	placed, err := r.readPlaced(ctx)
	if err != nil {
		return err
	}

	// Look up the types of the allowed nodes.
	allowed := make(map[string]struct{})
	for _, typeID := range r.policy.Snapshot().GetNodeTypeId() {
		allowed[typeID] = struct{}{}
	}
	wanted := make(map[string]struct{})
	for _, node := range placed {
		if _, ok := allowed[node.Node.GetTypeId()]; ok {
			wanted[node.Node.GetTypeId()] = struct{}{}
		}
	}
	if err := r.types.Hold(wanted); err != nil {
		return err
	}

	// Compile each node, then run every accepted node's entries together.
	entries := make(map[string]config.Config)
	reports := make([]*nodeReport, len(placed))
	for i, node := range placed {
		reports[i] = r.compile(node, allowed, entries)
	}
	if err := r.applier.Apply(entries); err != nil {
		return err
	}
	return r.project(ctx, reports)
}

// project writes the state of each reported node into the Device object,
// replacing the Flowgraph node capabilities it carried and keeping every other
// capability.
func (r *Reconciler) project(ctx context.Context, reports []*nodeReport) error {
	// Open the write transaction.
	tx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Load the Device object and require it to be this daemon's.
	obj, found, err := tx.GetObject(ctx, r.deviceKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrap(err, "get device object")
	}
	if !found {
		return world.ErrObjectNotFound
	}
	device, err := readDevice(ctx, obj)
	if err != nil {
		return errors.Wrap(err, "read device block")
	}
	if device.GetPeerId() != r.peerID {
		return errors.New("device object peer_id does not match this daemon")
	}

	// Keep the other capabilities in place and put the nodes after them.
	next := device.CloneVT()
	next.Capabilities = slices.DeleteFunc(next.Capabilities, func(capability *s4wave_device.DeviceCapability) bool {
		return capability.GetKind() == s4wave_device.DeviceCapabilityKindFlowgraphNode
	})
	for _, report := range reports {
		next.Capabilities = append(next.Capabilities, report.capability(r.applier))
	}
	if slices.EqualFunc(next.Capabilities, device.Capabilities, (*s4wave_device.DeviceCapability).EqualVT) {
		return nil
	}
	next.UpdatedAt = timestamppb.New(time.Now())
	if err := next.Validate(); err != nil {
		return err
	}

	// Write the Device block and commit.
	_, _, err = world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(next, true)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "write device block")
	}
	return errors.Wrap(tx.Commit(ctx), "commit")
}

// readPlaced reads the nodes placed on this Device from one World snapshot.
func (r *Reconciler) readPlaced(ctx context.Context) ([]*s4wave_flowgraph.PlacedFlowgraphNode, error) {
	tx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	return readPlacedNodes(ctx, tx, r.deviceKey, r.peerID)
}

// compile compiles node into entries keyed by their ConfigSet key. It reports
// the node rejected, and adds no entries, when its type is not allowed, its
// compile fails, or it emits a config ID its type did not declare.
func (r *Reconciler) compile(
	node *s4wave_flowgraph.PlacedFlowgraphNode,
	allowed map[string]struct{},
	entries map[string]config.Config,
) *nodeReport {
	// Reject a node whose type the policy does not allow.
	report := &nodeReport{node: node}
	typeID := node.Node.GetTypeId()
	if _, ok := allowed[typeID]; !ok {
		report.err = errors.Errorf("node type %q is not allowed by the Device policy", typeID)
		return report
	}

	// Wait for a controller to supply the type.
	report.nodeType = r.types.Get(typeID)
	if report.nodeType == nil {
		return report
	}
	compiled, err := report.nodeType.Compile(node)
	if err != nil {
		report.err = errors.Wrap(err, "compile node")
		return report
	}
	declared := report.nodeType.GetConfigIDs()
	for name, conf := range compiled {
		if !slices.Contains(declared, conf.GetConfigID()) {
			report.err = errors.Errorf("entry %q has undeclared config ID %q", name, conf.GetConfigID())
			return report
		}
	}

	// Name each entry for its Flowgraph, node and entry, so nodes of different
	// Flowgraphs never collide.
	for name, conf := range compiled {
		key := node.FlowgraphKey + "/" + node.NodeID + "/" + name
		entries[key] = conf
		report.keys = append(report.keys, key)
	}
	slices.Sort(report.keys)
	return report
}
