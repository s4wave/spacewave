package kernel

import "slices"

// epollSet is an epoll file's interest list. Like Linux, it keys entries by
// descriptor and file, and drops an entry once its file closes everywhere.
// The kernel lock guards it.
type epollSet struct {
	// entries is the interest list. An entry that reports moves to the end,
	// so a wait that returns fewer events than are ready reaches the rest
	// next.
	entries []*epollEntry
}

// find returns the index of the entry for fd and of, or -1.
func (s *epollSet) find(fd uint32, of *OpenFile) int {
	return slices.IndexFunc(s.entries, func(e *epollEntry) bool {
		return e.fd == fd && e.of == of
	})
}

// poll returns up to limit events that have happened, and the channels that
// close at the next change of the entries with none.
func (s *epollSet) poll(limit int) ([]epollEvent, []<-chan struct{}) {
	// Drop the entries whose files closed.
	s.entries = slices.DeleteFunc(s.entries, func(e *epollEntry) bool {
		return e.of.refs == 0
	})

	// Poll each entry until limit entries report.
	var events []epollEvent
	var changes []<-chan struct{}
	var reported, rest []*epollEntry
	for _, e := range s.entries {
		if len(events) == limit {
			rest = append(rest, e)
			continue
		}
		revents, entryChanges := e.poll()
		if revents == 0 {
			changes = append(changes, entryChanges...)
			rest = append(rest, e)
			continue
		}
		events = append(events, epollEvent{events: revents, data: e.data})
		reported = append(reported, e)
	}
	s.entries = append(rest, reported...)
	return events, changes
}

// epollEvent is an event an epoll wait reports.
type epollEvent struct {
	// events is the events that happened.
	events uint32
	// data is the entry's data.
	data uint64
}
