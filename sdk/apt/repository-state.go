package s4wave_apt

import "github.com/pkg/errors"

// Validate checks the repository state is known.
func (s AptRepositoryState) Validate() error {
	switch s {
	case AptRepositoryState_AptRepositoryState_EMPTY:
	case AptRepositoryState_AptRepositoryState_INDEXING:
	case AptRepositoryState_AptRepositoryState_READY:
	case AptRepositoryState_AptRepositoryState_ERROR:
	default:
		return errors.Wrap(ErrUnknownAptRepositoryState, s.String())
	}
	return nil
}

// CanTransitionTo returns true if the repository can move to the next state.
func (s AptRepositoryState) CanTransitionTo(next AptRepositoryState) bool {
	switch s {
	case AptRepositoryState_AptRepositoryState_EMPTY:
		return next == AptRepositoryState_AptRepositoryState_INDEXING
	case AptRepositoryState_AptRepositoryState_INDEXING:
		return next == AptRepositoryState_AptRepositoryState_READY ||
			next == AptRepositoryState_AptRepositoryState_ERROR
	case AptRepositoryState_AptRepositoryState_READY:
		return next == AptRepositoryState_AptRepositoryState_INDEXING
	case AptRepositoryState_AptRepositoryState_ERROR:
		return next == AptRepositoryState_AptRepositoryState_INDEXING
	default:
		return false
	}
}

// EnsureTransitionTo returns an error if the repository transition is illegal.
func (s AptRepositoryState) EnsureTransitionTo(next AptRepositoryState) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if !s.CanTransitionTo(next) {
		return errors.Wrapf(
			ErrInvalidAptRepositoryStateTransition,
			"%s -> %s",
			s.String(),
			next.String(),
		)
	}
	return nil
}

// TransitionState transitions the repository to the next legal state.
func (r *AptRepository) TransitionState(next AptRepositoryState) error {
	if err := r.GetState().EnsureTransitionTo(next); err != nil {
		return err
	}
	prev := r.State
	r.State = next
	if err := r.Validate(); err != nil {
		r.State = prev
		return err
	}
	return nil
}
