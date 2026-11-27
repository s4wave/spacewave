package resource_world

import (
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// Mount assembles World Resources under one granting component's dependencies.
// Each constructed Resource retains its own capability and release behavior.
type Mount struct {
	// le logs Resource and controller lifecycle failures.
	le *logrus.Entry
	// b resolves World and typed handler directives.
	b bus.Bus
	// lookupOp resolves operations under this mount's authority.
	lookupOp world.LookupOp
	// options carries trusted access configuration into descendants.
	options []WorldStateResourceOption
}

// NewMount captures the dependencies and trusted options used to assemble Resources.
func NewMount(le *logrus.Entry, b bus.Bus, lookupOp world.LookupOp, opts ...WorldStateResourceOption) *Mount {
	return &Mount{le: le, b: b, lookupOp: lookupOp, options: opts}
}

// NewEngineResource assembles Engine, snapshot, transaction and typed access.
func (m *Mount) NewEngineResource(engine world.Engine, info *s4wave_world.EngineInfo) *EngineResource {
	// Bind the registry scope before supplying descendant constructors.
	if info != nil {
		m = NewMount(m.le, m.b, m.lookupOp, append(slices.Clone(m.options), WithEngineID(info.GetEngineId()))...)
	}

	// Supply descendant Engine registration to the lower RPC wrappers.
	return newEngineResource(m.le, m.b, engine, m.lookupOp, info, m.addEngine, m.options...)
}

// NewWorldStateResource assembles snapshot RPCs with inherited mount authority.
func (m *Mount) NewWorldStateResource(ws world.WorldState) *WorldStateResource {
	return newWorldStateResource(m.le, m.b, ws, m.lookupOp, srpc.NewMux(), nil, false, m.addEngine, newMountOptions(m.options...))
}

// NewEngineWorldStateResource assembles snapshot and typed RPCs with live storage.
func (m *Mount) NewEngineWorldStateResource(ws world.WorldState, engine world.Engine) *WorldStateResource {
	// Apply the trusted write policy before passing live storage to the snapshot.
	access := newMountOptions(m.options...)
	engine = world.GuardEngine(engine, access.writeGuard)
	return newWorldStateResource(m.le, m.b, ws, m.lookupOp, srpc.NewMux(), engine, true, m.addEngine, access)
}

// NewTxResource assembles transaction RPCs, optionally with live typed access.
func (m *Mount) NewTxResource(tx world.Tx, engine world.Engine) *TxResource {
	return newTxResource(m.le, m.b, tx, m.lookupOp, engine, m.addEngine, m.options...)
}

// addEngine transfers a newly assembled Engine Resource to the requesting client.
// The caller supplies the full inherited options, including nested authority.
func (m *Mount) addEngine(client resource_server.ResourceClientContext, engine world.Engine, opts ...WorldStateResourceOption) (uint32, error) {
	// Transfer the mount to the client, closing it if adoption fails.
	resource := NewMount(m.le, m.b, m.lookupOp, opts...).NewEngineResource(engine, nil)
	id, err := client.AddResource(resource.GetMux(), resource.Close)
	if err != nil {
		resource.Close()
		return 0, err
	}
	return id, nil
}
