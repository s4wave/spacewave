package device_flowgraph

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/peer"
)

// configApplier applies the entries of every placed node as one ConfigSet on
// the Device Session's bus and reports each entry's controller state. The
// Device peer and its links live on that bus, so the node controllers must run
// there to open and serve peer streams.
//
// Watch, Apply and Release run on the reconciler's loop goroutine. The
// directive callbacks run on bus goroutines and share only the Session
// transport and the entry states.
type configApplier struct {
	// b is the bus the Session transport is looked up on.
	b bus.Bus
	// notify wakes the reconciler when the Session transport or an entry's state
	// changes.
	notify func()
	// lookup releases the watch on the Session transport, or is nil before Watch.
	lookup directive.Reference

	// mtx guards session.
	mtx sync.Mutex
	// session is the running Session transport, or nil while none runs.
	session *transport.SessionTransport
	// sessionBus is the Session bus current is applied on, or nil while no
	// Session runs. A restarted transport publishes a new bus.
	sessionBus bus.Bus
	// current is the applied ConfigSet, or nil while no entry is applied.
	current *appliedConfigSet
	// rev is the revision last assigned to an entry. A controller restarts
	// only for a greater revision, so an entry whose config is unchanged keeps
	// its revision and keeps running.
	rev uint64
}

// appliedConfigSet is a ConfigSet held by one ApplyConfigSet directive.
type appliedConfigSet struct {
	// set is the applied ConfigSet.
	set configset.ConfigSet
	// ref releases the directive and with it the entries no other set holds.
	ref directive.Reference
	// retirePrevious releases the set this one replaced, once. The
	// replaced set stays held until this one reports a state for every
	// entry, because the ConfigSet controller stops an entry that no directive
	// holds before the new directive resolves.
	retirePrevious func()

	// mtx guards states.
	mtx sync.Mutex
	// states contains the state of each entry by attached value ID. The
	// ConfigSet controller replaces an entry's value when its state changes.
	states map[uint32]configset.State
}

// newConfigApplier constructs a configApplier that wakes notify on every state
// change.
func newConfigApplier(b bus.Bus, notify func()) *configApplier {
	return &configApplier{b: b, notify: notify}
}

// Watch follows the running transport of the Session with peer ID id, and wakes
// the reconciler as it starts, stops or restarts.
func (a *configApplier) Watch(id peer.ID) error {
	_, ref, err := bus.ExecOneOffWatchCb(
		func(val directive.TypedAttachedValue[*transport.SessionTransport]) bool {
			// Follow the lookup as the transport starts and stops.
			a.mtx.Lock()
			if val == nil {
				a.session = nil
			} else {
				a.session = val.GetValue()
			}
			a.mtx.Unlock()
			a.notify()
			return true
		},
		a.b,
		transport.NewLookupSessionTransport(id),
	)
	if err != nil {
		return err
	}
	a.lookup = ref
	return nil
}

// Apply makes entries, keyed by ConfigSet key, the applied ConfigSet on the
// Session bus. It does nothing when the entries equal the applied ones. While
// no Session runs the entries wait, and a Session bus that replaces the one
// the ConfigSet runs on gets the entries applied again.
func (a *configApplier) Apply(entries map[string]config.Config) error {
	// Release the entries of a Session bus that stopped or was replaced.
	sessionBus := a.currentBus()
	if sessionBus != a.sessionBus {
		a.current.release()
		a.current = nil
		a.sessionBus = sessionBus
	}
	if sessionBus == nil {
		return nil
	}

	// Keep the revision of each unchanged entry and advance each changed one.
	next := make(configset.ConfigSet, len(entries))
	for key, conf := range entries {
		next[key] = a.entry(key, conf)
	}
	if a.current == nil && len(next) == 0 {
		return nil
	}
	if a.current != nil && a.current.set.Equal(next) {
		return nil
	}

	// Hold the old set until the new one runs, so an unchanged entry keeps
	// its controller. With no new set, nothing needs the old one.
	var applied *appliedConfigSet
	if len(next) != 0 {
		var err error
		applied, err = a.add(next, a.current)
		if err != nil {
			return err
		}
	} else {
		a.current.release()
	}
	a.current = applied
	return nil
}

// entry returns the ConfigSet entry for conf, reusing the applied entry when
// its config is unchanged.
func (a *configApplier) entry(key string, conf config.Config) configset.ControllerConfig {
	if a.current != nil {
		if prev, ok := a.current.set[key]; ok && prev.GetConfig().EqualsConfig(conf) {
			return prev
		}
	}
	a.rev++
	return configset.NewControllerConfig(a.rev, conf)
}

// add applies set with a new ApplyConfigSet directive that replaces previous.
func (a *configApplier) add(set configset.ConfigSet, previous *appliedConfigSet) (*appliedConfigSet, error) {
	// Track each entry's state as the ConfigSet controller reports it.
	applied := &appliedConfigSet{
		set:            set,
		retirePrevious: sync.OnceFunc(previous.release),
		states:         make(map[uint32]configset.State),
	}
	_, ref, err := a.sessionBus.AddDirective(
		configset.NewApplyConfigSet(set),
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				// Take only the states the ConfigSet controller reports.
				state, ok := av.GetValue().(configset.State)
				if !ok {
					return
				}

				// Record the state, retire the replaced set once every entry
				// reports, and wake the reconciler to project the state.
				applied.mtx.Lock()
				applied.states[av.GetValueID()] = state
				covered := applied.coversSet()
				applied.mtx.Unlock()
				if covered {
					applied.retirePrevious()
				}
				a.notify()
			},
			func(av directive.AttachedValue) {
				applied.mtx.Lock()
				delete(applied.states, av.GetValueID())
				applied.mtx.Unlock()
			},
			nil,
		),
	)
	if err != nil {
		return nil, err
	}
	applied.ref = ref
	return applied, nil
}

// coversSet reports whether every entry of the set has a state. The caller
// holds mtx.
func (s *appliedConfigSet) coversSet() bool {
	reported := make(map[string]struct{}, len(s.states))
	for _, state := range s.states {
		reported[state.GetId()] = struct{}{}
	}
	return len(reported) == len(s.set)
}

// release releases the set and the set it replaced, if that is still held.
// It does nothing for a nil set.
func (s *appliedConfigSet) release() {
	if s == nil {
		return
	}
	s.retirePrevious()
	s.ref.Release()
}

// currentBus returns the bus of the running Session transport, or nil while none
// runs.
func (a *configApplier) currentBus() bus.Bus {
	a.mtx.Lock()
	session := a.session
	a.mtx.Unlock()
	if session == nil {
		return nil
	}
	return session.GetChildBus()
}

// State returns the state of the entry with the ConfigSet key, or nil while
// the ConfigSet controller has not reported it.
func (a *configApplier) State(key string) configset.State {
	// Report no state while no ConfigSet is applied.
	if a.current == nil {
		return nil
	}

	// Find the entry among the states the controller reported.
	a.current.mtx.Lock()
	defer a.current.mtx.Unlock()
	for _, state := range a.current.states {
		if state.GetId() == key {
			return state
		}
	}
	return nil
}

// Release releases the Session watch and the applied ConfigSet.
func (a *configApplier) Release() {
	if a.lookup != nil {
		a.lookup.Release()
		a.lookup = nil
	}
	a.current.release()
	a.current = nil
}
