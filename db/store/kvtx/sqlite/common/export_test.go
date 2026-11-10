package common

import (
	"context"
	"time"
)

// SetReadTurns gives s's snapshot reads n turns.
func SetReadTurns[T SQLiteDriverConfig](s *Store[T], n int) {
	s.readTurns = make(readTurns, n)
}

// RunPunchStep runs one freelist punch round on s as its checkpointer would.
// The round runs whatever the interval since the last.
func RunPunchStep[T SQLiteDriverConfig](ctx context.Context, s *Store[T]) {
	// Clear the last round's time so the interval has passed.
	s.punchMtx.Lock()
	s.punchLast = time.Time{}
	s.punchMtx.Unlock()

	// Run the round as the checkpointer does.
	s.punchStep(ctx)
}

// SetPunchHook sets the hook s's punch rounds run at each named stage.
func SetPunchHook[T SQLiteDriverConfig](s *Store[T], hook func(stage string) error) {
	s.punchMtx.Lock()
	s.punchHook = hook
	s.punchMtx.Unlock()
}
