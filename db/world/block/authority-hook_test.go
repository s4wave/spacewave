package world_block

import (
	"context"
	"testing"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// hookAuthority wraps an Authority and runs admit before each Admit its
// turns delegate, so a test can hold a commit at the moment it is proposed.
type hookAuthority struct {
	world.Authority
	// admit runs before the wrapped Admit, or is nil. An error refuses the
	// candidate.
	admit func(ctx context.Context, candidate world.Candidate) error
}

// Begin opens the wrapped turn and hooks its Admit.
func (a *hookAuthority) Begin(ctx context.Context) (world.Turn, error) {
	turn, err := a.Authority.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &hookTurn{Turn: turn, a: a}, nil
}

// hookTurn is a turn of a hookAuthority.
type hookTurn struct {
	world.Turn
	// a is the authority.
	a *hookAuthority
}

// Admit runs the hook, then the wrapped Admit.
func (t *hookTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	if t.a.admit != nil {
		if err := t.a.admit(ctx, candidate); err != nil {
			return nil, err
		}
	}
	return t.Turn.Admit(ctx, candidate)
}

// newTestAuthority returns the local authority of the World root name in
// base's bucket, without a coordinator.
func newTestAuthority(t testing.TB, le *logrus.Entry, base *bucket_lookup.Cursor, name string) *LocalAuthority {
	t.Helper()
	return NewLocalAuthority(le, base, name, nil, coord.Scope{})
}

// _ is a type assertion
var _ world.Authority = (*hookAuthority)(nil)
