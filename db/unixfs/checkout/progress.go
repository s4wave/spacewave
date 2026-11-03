package unixfs_checkout

import (
	"time"

	"github.com/sirupsen/logrus"
)

// Phase is the step a checkout is in.
type Phase int

const (
	// PhaseChecking looks for a complete checkout to reuse.
	PhaseChecking Phase = iota
	// PhaseWaiting waits for another process writing the same checkout.
	PhaseWaiting
	// PhaseFetching reads the blocks the checkout lacks.
	PhaseFetching
	// PhaseAssembling writes the files from the fetched blocks and checks
	// them.
	PhaseAssembling
)

// Progress is a checkout's phase and the bytes it counts.
type Progress struct {
	// Phase is the step the checkout is in.
	Phase Phase
	// Done is the bytes of the files held so far: fetched, or read from the
	// previous checkout or an interrupted attempt, while fetching, and
	// written while assembling. It is zero while checking or waiting.
	Done uint64
	// Total is the size of the files found so far. While fetching it grows
	// as the walk discovers files and then holds; while assembling it is the
	// fixed size of the files being written.
	Total uint64
}

// phaseClock adds up the time a checkout spends in each phase, for its log.
type phaseClock struct {
	// phase is the current phase, entered at since.
	phase Phase
	since time.Time
	// spent is the time each earlier phase took, by phase.
	spent [PhaseAssembling + 1]time.Duration
}

// newPhaseClock starts timing at the checking phase.
func newPhaseClock() *phaseClock {
	return &phaseClock{since: time.Now()}
}

// enter charges the time since the last phase change to that phase and
// starts timing phase.
func (c *phaseClock) enter(phase Phase) {
	now := time.Now()
	c.spent[c.phase] += now.Sub(c.since)
	c.phase, c.since = phase, now
}

// fields closes the current phase and describes the time each phase took.
func (c *phaseClock) fields() logrus.Fields {
	c.enter(c.phase)
	fields := logrus.Fields{}
	for phase, name := range [...]string{"checking", "waiting", "fetching", "assembling"} {
		if c.spent[phase] != 0 {
			fields[name] = c.spent[phase].Round(time.Millisecond).String()
		}
	}
	return fields
}
