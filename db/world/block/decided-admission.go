package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// decidedAdmission is the Admission of an Authority that decides at Admit:
// an accepted candidate is durable at once, and a superseded or rejected one
// was never stored.
type decidedAdmission struct {
	// seqno is the World seqno at the candidate's root.
	seqno uint64
	// decision is the Authority's answer.
	decision world.Decision
	// head is the head the Authority held when it decided.
	head world.Head
	// err is the rejection's reason, nil otherwise.
	err error
}

// Seqno returns the World seqno at the candidate's root.
func (a *decidedAdmission) Seqno() uint64 { return a.seqno }

// Head returns the head the Authority held when it decided.
func (a *decidedAdmission) Head() world.Head { return a.head }

// WaitDecided returns the decision.
func (a *decidedAdmission) WaitDecided(context.Context) (world.Decision, error) {
	return a.decision, a.err
}

// WaitDurable returns the candidate's seqno once accepted, or why it was not.
func (a *decidedAdmission) WaitDurable(context.Context) (uint64, error) {
	switch a.decision {
	case world.Accepted:
		return a.seqno, nil
	case world.Superseded:
		return 0, world.ErrSuperseded
	default:
		return 0, a.err
	}
}

// Release does nothing: a decided candidate holds nothing it could release.
func (a *decidedAdmission) Release() {}

// _ is a type assertion
var _ world.Admission = (*decidedAdmission)(nil)
