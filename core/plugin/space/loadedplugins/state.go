package loadedplugins

import (
	"slices"

	"github.com/aperturerobotics/util/broadcast"
)

// State tracks active Space plugin loads.
type State struct {
	bcast broadcast.Broadcast
	ids   []string
}

// GetAndWaitCh returns loaded plugin IDs and a channel closed when they change.
func (s *State) GetAndWaitCh() ([]string, <-chan struct{}) {
	var ids []string
	var ch <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ids = slices.Clone(s.ids)
		ch = getWaitCh()
	})
	return ids, ch
}

// Set replaces loaded plugin IDs and wakes watchers when the value changes.
func (s *State) Set(ids []string) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if slices.Equal(s.ids, ids) {
			return
		}
		s.ids = slices.Clone(ids)
		broadcast()
	})
}
