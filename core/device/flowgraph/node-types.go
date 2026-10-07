package device_flowgraph

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// nodeTypes holds one LookupFlowgraphNodeType directive for each node type a
// placed node uses, so a type supplied by a controller that loads later
// resolves while the Flowgraph waits.
//
// Hold and Release run on the reconciler's loop goroutine. The resolver
// callbacks run on bus goroutines and share only the resolved values.
type nodeTypes struct {
	// b is the bus the lookup directives run on.
	b bus.Bus
	// notify wakes the reconciler when a lookup resolves or changes.
	notify func()
	// held contains the lookup of each node type in use, by type ID.
	held map[string]*heldNodeType

	// mtx guards the nodeType of every held lookup.
	mtx sync.Mutex
}

// heldNodeType is a running lookup of one node type.
type heldNodeType struct {
	// ref releases the lookup directive.
	ref directive.Reference
	// nodeType is the resolved node type, or nil while no controller supplies it.
	nodeType s4wave_flowgraph.FlowgraphNodeType
}

// newNodeTypes constructs a nodeTypes that wakes notify on every change.
func newNodeTypes(b bus.Bus, notify func()) *nodeTypes {
	return &nodeTypes{b: b, notify: notify, held: make(map[string]*heldNodeType)}
}

// Hold looks up the wanted node types and releases the lookup of every other
// type.
func (n *nodeTypes) Hold(wanted map[string]struct{}) error {
	// Release the types no placed node uses.
	for typeID, held := range n.held {
		if _, ok := wanted[typeID]; !ok {
			held.ref.Release()
			delete(n.held, typeID)
		}
	}

	// Start a lookup for each new type.
	for typeID := range wanted {
		if _, ok := n.held[typeID]; ok {
			continue
		}
		// Hold a lookup that follows the controller supplying the type.
		held := &heldNodeType{}
		_, ref, err := bus.ExecOneOffWatchCb(
			func(val directive.TypedAttachedValue[s4wave_flowgraph.LookupFlowgraphNodeTypeValue]) bool {
				// Follow the lookup as controllers supply and withdraw the type.
				n.mtx.Lock()
				if val == nil {
					held.nodeType = nil
				} else {
					held.nodeType = val.GetValue()
				}
				n.mtx.Unlock()
				n.notify()
				return true
			},
			n.b,
			s4wave_flowgraph.NewLookupFlowgraphNodeType(typeID),
		)
		if err != nil {
			return err
		}
		held.ref = ref
		n.held[typeID] = held
	}
	return nil
}

// Get returns the resolved node type, or nil while no controller supplies it.
func (n *nodeTypes) Get(typeID string) s4wave_flowgraph.FlowgraphNodeType {
	// Return nothing for a type no lookup holds.
	held, ok := n.held[typeID]
	if !ok {
		return nil
	}

	// Read the type the lookup resolved.
	n.mtx.Lock()
	defer n.mtx.Unlock()
	return held.nodeType
}

// Release releases every lookup.
func (n *nodeTypes) Release() {
	for typeID, held := range n.held {
		held.ref.Release()
		delete(n.held, typeID)
	}
}
