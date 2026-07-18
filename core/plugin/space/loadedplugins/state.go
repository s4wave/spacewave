package loadedplugins

import (
	"slices"

	"github.com/aperturerobotics/util/broadcast"
)

// State tracks demanded Space plugins, their running state, and whether each
// initial capability-registration pass reached a terminal state.
type State struct {
	bcast      broadcast.Broadcast
	desired    []string
	running    map[string]bool
	terminal   map[string]bool
	reconciled bool
}

// GetAndWaitCh returns running, registration-complete plugin IDs and a channel
// closed when the state changes.
func (s *State) GetAndWaitCh() ([]string, <-chan struct{}) {
	var ids []string
	var ch <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ids = s.runningIDsLocked()
		ch = getWaitCh()
	})
	return ids, ch
}

// HasPendingAndWaitCh reports whether desired plugin registration is pending
// and returns a channel closed when the state changes.
func (s *State) HasPendingAndWaitCh() (bool, <-chan struct{}) {
	var pending bool
	var ch <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		pending = !s.reconciled
		if !pending {
			for _, id := range s.desired {
				if !s.terminal[id] {
					pending = true
					break
				}
			}
		}
		ch = getWaitCh()
	})
	return pending, ch
}

// Reconcile replaces the desired plugin IDs and marks initial reconciliation
// complete.
func (s *State) Reconcile(ids []string) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		changed := !s.reconciled || !slices.Equal(s.desired, ids)
		s.reconciled = true
		s.desired = slices.Clone(ids)
		desired := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			desired[id] = struct{}{}
		}
		for id := range s.running {
			if _, ok := desired[id]; !ok {
				delete(s.running, id)
				changed = true
			}
		}
		for id := range s.terminal {
			if _, ok := desired[id]; !ok {
				delete(s.terminal, id)
				changed = true
			}
		}
		if changed {
			broadcast()
		}
	})
}

// SetPluginState atomically records whether a desired plugin is running and
// whether its initial registration reached a terminal complete or failed state.
func (s *State) SetPluginState(id string, running, terminal bool) {
	if id == "" {
		return
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !slices.Contains(s.desired, id) {
			return
		}
		if s.running == nil {
			s.running = make(map[string]bool)
		}
		if s.terminal == nil {
			s.terminal = make(map[string]bool)
		}
		if s.running[id] == running && s.terminal[id] == terminal {
			return
		}
		s.running[id] = running
		s.terminal[id] = terminal
		broadcast()
	})
}

// Reset clears desired and observed plugin state.
func (s *State) Reset() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !s.reconciled && len(s.desired) == 0 && len(s.running) == 0 && len(s.terminal) == 0 {
			return
		}
		s.desired = nil
		s.running = nil
		s.terminal = nil
		s.reconciled = false
		broadcast()
	})
}

func (s *State) runningIDsLocked() []string {
	ids := make([]string, 0, len(s.desired))
	for _, id := range s.desired {
		if s.running[id] && s.terminal[id] {
			ids = append(ids, id)
		}
	}
	return ids
}
