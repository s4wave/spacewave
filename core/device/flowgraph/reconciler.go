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
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Reconciler runs the Flowgraph nodes placed on one Device. It watches the
// World for placements and the DevicePolicy for the allowed node types,
// compiles each allowed node through its node type, applies the compiled
// entries as one ConfigSet on the Device Session's bus, and writes each node's
// state into the Device object. A node that fails shows as rejected with its
// error while the other nodes keep running. The daemon is the only writer of
// the Device object.
type Reconciler struct {
	le *logrus.Entry
	// b is the root bus node types resolve on. The ConfigSet applies on the Device
	// Session's bus, which the Session transport looked up on b publishes.
	b bus.Bus
	// engine is the World engine of the Space holding the Device.
	engine world.Engine
	// policy supplies the allowed node types.
	policy *device_policy.PolicyStore
	// forgeWorker receives the declaration of the accepted forge-worker node.
	forgeWorker *ForgeWorkerWatch
	// deviceKey is the object key of this daemon's Device.
	deviceKey string
	// peerID is the peer ID of this daemon's Device.
	peerID string
	// hold keeps the daemon alive and returns its release. The Reconciler
	// holds while it runs at least one node, since a node serves with no
	// client attached.
	hold func() func()
	// release releases the hold, or is nil while no node runs.
	release func()

	// wake holds a pending reconcile. Every watched source sets it.
	wake chan struct{}
	// types holds the lookup of each node type in use.
	types *nodeTypes
	// applier applies the compiled entries.
	applier *configApplier
}

// NewReconciler constructs a Reconciler for the Device at deviceKey with the
// given peer ID. It calls hold while a node runs and calls the release it
// returns once none do.
func NewReconciler(
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	policy *device_policy.PolicyStore,
	forgeWorker *ForgeWorkerWatch,
	deviceKey, peerID string,
	hold func() func(),
) *Reconciler {
	r := &Reconciler{
		le:          le,
		b:           b,
		engine:      engine,
		policy:      policy,
		forgeWorker: forgeWorker,
		deviceKey:   deviceKey,
		peerID:      peerID,
		hold:        hold,
		wake:        make(chan struct{}, 1),
	}
	r.types = newNodeTypes(b, r.notify)
	r.applier = newConfigApplier(b, r.notify)
	return r
}

// Run reconciles until ctx ends, then releases every node's configs.
func (r *Reconciler) Run(ctx context.Context) error {
	// Release the lookups, the configs, and the hold on every exit.
	defer r.types.Release()
	defer r.applier.Release()
	defer r.setHeld(false)

	// Follow the Device Session's transport, which hosts the node controllers.
	sessionPeerID, err := peer.IDB58Decode(r.peerID)
	if err != nil {
		return errors.Wrap(err, "parse device peer id")
	}
	if err := r.applier.Watch(sessionPeerID); err != nil {
		return err
	}

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
			err := r.reconcile(egCtx)
			switch {
			case err == nil || egCtx.Err() != nil:
			case errors.Is(err, world.ErrObjectNotFound):
				// The Device object arrives with a later World change.
				r.le.WithError(err).Debug("device object not available for flowgraph nodes")
			default:
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
	// Compile the placed nodes, then run every accepted node's entries together.
	entries, reports, err := r.compilePlaced(ctx)
	if err != nil {
		return err
	}
	if err := r.applier.Apply(entries); err != nil {
		return err
	}
	r.setHeld(len(entries) != 0)
	r.forgeWorker.Set(r.deviceKey, acceptedForgeWorker(reports))
	return r.project(ctx, reports)
}

// compilePlaced compiles each node placed on this Device from one World
// snapshot. It returns the entries of the accepted nodes by ConfigSet key and a
// report for every node.
func (r *Reconciler) compilePlaced(ctx context.Context) (map[string]config.Config, []*nodeReport, error) {
	// Read the nodes placed on this Device.
	tx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	placed, err := readPlacedNodes(ctx, tx, r.deviceKey, r.peerID)
	if err != nil {
		return nil, nil, err
	}

	// Look up the types of the allowed nodes.
	pass := &compilePass{
		allowed: make(map[string]struct{}),
		entries: make(map[string]config.Config),
		claims:  make(map[string]string),
	}
	for _, typeID := range r.policy.Snapshot().GetNodeTypeId() {
		pass.allowed[typeID] = struct{}{}
	}
	wanted := make(map[string]struct{})
	for _, node := range placed {
		if _, ok := pass.allowed[node.Node.GetTypeId()]; ok {
			wanted[node.Node.GetTypeId()] = struct{}{}
		}
	}
	if err := r.types.Hold(wanted); err != nil {
		return nil, nil, err
	}

	// Compile each node in order.
	reports := make([]*nodeReport, len(placed))
	for i, node := range placed {
		reports[i] = r.compile(ctx, tx, node, pass)
	}
	return pass.entries, reports, nil
}

// acceptedForgeWorker returns the declaration of the accepted forge-worker node,
// or nil when no such node is accepted. Only an accepted node has a shown
// capability, and a Device keeps one Forge Worker, so at most one report
// carries a declaration.
func acceptedForgeWorker(reports []*nodeReport) *s4wave_device.ForgeWorkerDeclaration {
	for _, report := range reports {
		if declaration := report.shown.GetWorkerDeclaration(); declaration != nil {
			return declaration
		}
	}
	return nil
}

// setHeld holds the daemon while held is true and releases it otherwise.
func (r *Reconciler) setHeld(held bool) {
	switch {
	case held && r.release == nil:
		r.release = r.hold()
	case !held && r.release != nil:
		r.release()
		r.release = nil
	}
}

// project writes the state of each reported node into the Device object,
// replacing the Flowgraph node capabilities it carried and keeping every other
// capability. It opens a write transaction only when the Device differs.
func (r *Reconciler) project(ctx context.Context, reports []*nodeReport) error {
	// Compare against the current Device first, since most wakes change nothing.
	current, err := r.readDeviceState(ctx)
	if err != nil {
		return err
	}
	if slices.EqualFunc(r.nextCapabilities(current, reports), current.GetCapabilities(), (*s4wave_device.DeviceCapability).EqualVT) {
		return nil
	}

	// Open the write transaction.
	tx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Load the Device again, since it can change between the transactions.
	obj, device, err := r.loadDevice(ctx, tx)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	next := device.CloneVT()
	next.Capabilities = r.nextCapabilities(device, reports)
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

// nextCapabilities returns the capabilities of device with its Flowgraph node
// capabilities replaced by the reports. The other capabilities keep their place
// and the nodes follow them.
func (r *Reconciler) nextCapabilities(device *s4wave_device.Device, reports []*nodeReport) []*s4wave_device.DeviceCapability {
	capabilities := slices.DeleteFunc(slices.Clone(device.GetCapabilities()), func(capability *s4wave_device.DeviceCapability) bool {
		return s4wave_flowgraph.IsNodeCapabilityID(capability.GetId())
	})
	for _, report := range reports {
		capabilities = append(capabilities, report.capability(r.applier))
	}
	return capabilities
}

// readDeviceState reads this daemon's Device on a read transaction.
func (r *Reconciler) readDeviceState(ctx context.Context) (*s4wave_device.Device, error) {
	// Read the Device inside a transaction that is discarded on return.
	tx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Release the object state, since the caller needs only the Device.
	obj, device, err := r.loadDevice(ctx, tx)
	world.ReleaseObjectState(obj)
	return device, err
}

// loadDevice loads this daemon's Device object from tx and requires it to be
// this daemon's. The caller releases the returned object state.
func (r *Reconciler) loadDevice(ctx context.Context, tx world.WorldState) (world.ObjectState, *s4wave_device.Device, error) {
	// Find the Device object.
	obj, found, err := tx.GetObject(ctx, r.deviceKey)
	if err != nil {
		return obj, nil, errors.Wrap(err, "get device object")
	}
	if !found {
		return obj, nil, world.ErrObjectNotFound
	}

	// Read the Device and require it to belong to this daemon.
	device, err := readDevice(ctx, obj)
	if err != nil {
		return obj, nil, errors.Wrap(err, "read device block")
	}
	if device.GetPeerId() != r.peerID {
		return obj, nil, errors.New("device object peer_id does not match this daemon")
	}
	return obj, device, nil
}

// compilePass is the state shared by the nodes compiled in one reconcile.
type compilePass struct {
	// allowed contains the node type IDs the policy allows.
	allowed map[string]struct{}
	// entries contains the entries of the accepted nodes by ConfigSet key.
	entries map[string]config.Config
	// claims contains the Flowgraph key and node ID of the node that shows each
	// capability kind and checkout root name, keyed by kind and name.
	claims map[string]string
}

// compile compiles node into the pass's entries, keyed by their ConfigSet key.
// It reports the node rejected, and adds no entries, when its type is not
// allowed, its compile fails, it emits a config ID its type did not declare, its
// capability is refused, or an earlier node already shows the same capability.
func (r *Reconciler) compile(
	ctx context.Context,
	ws world.WorldState,
	node *s4wave_flowgraph.PlacedFlowgraphNode,
	pass *compilePass,
) *nodeReport {
	// Reject a node whose type the policy does not allow.
	report := &nodeReport{node: node}
	typeID := node.Node.GetTypeId()
	if _, ok := pass.allowed[typeID]; !ok {
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

	// Show the capability the node type supplies, once no earlier node shows it.
	if source, ok := report.nodeType.(s4wave_flowgraph.FlowgraphNodeCapability); ok {
		shown, err := source.GetCapability(ctx, ws, node)
		if err != nil {
			report.err = errors.Wrap(err, "get node capability")
			return report
		}
		claim := shown.ClaimKey()
		if kept, ok := pass.claims[claim]; ok {
			report.err = errors.Errorf("%s is already provided by node %s", shown.GetLabel(), kept)
			return report
		}
		pass.claims[claim] = node.FlowgraphKey + "/" + node.NodeID
		report.shown = shown
	}

	// Name each entry for its Flowgraph, node and entry, so nodes of different
	// Flowgraphs never collide.
	for name, conf := range compiled {
		key := node.FlowgraphKey + "/" + node.NodeID + "/" + name
		pass.entries[key] = conf
		report.keys = append(report.keys, key)
	}
	slices.Sort(report.keys)
	return report
}
