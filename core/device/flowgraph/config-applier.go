package device_flowgraph

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
)

// configApplier applies the entries of every placed node as one ConfigSet and
// reports each entry's controller state.
//
// Apply and Release run on the reconciler's loop goroutine. The directive
// callbacks run on bus goroutines and share only the entry states.
type configApplier struct {
	// b is the bus the ConfigSet applies on.
	b bus.Bus
	// notify wakes the reconciler when an entry's state changes.
	notify func()
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

// Apply makes entries, keyed by ConfigSet key, the applied ConfigSet. It does
// nothing when the entries equal the applied ones.
func (a *configApplier) Apply(entries map[string]config.Config) error {
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
	_, ref, err := a.b.AddDirective(
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

// Release releases the applied ConfigSet.
func (a *configApplier) Release() {
	a.current.release()
	a.current = nil
}
