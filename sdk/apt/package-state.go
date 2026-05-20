package s4wave_apt

import "github.com/pkg/errors"

// Validate checks the package state is known.
func (s AptPackageState) Validate() error {
	switch s {
	case AptPackageState_AptPackageState_IMPORTING:
	case AptPackageState_AptPackageState_BUILT:
	case AptPackageState_AptPackageState_PUBLISHED:
	case AptPackageState_AptPackageState_SUPERSEDED:
	default:
		return errors.Wrap(ErrUnknownAptPackageState, s.String())
	}
	return nil
}

// CanTransitionTo returns true if the package can move to the next state.
func (s AptPackageState) CanTransitionTo(next AptPackageState) bool {
	switch s {
	case AptPackageState_AptPackageState_IMPORTING:
		return next == AptPackageState_AptPackageState_BUILT
	case AptPackageState_AptPackageState_BUILT:
		return next == AptPackageState_AptPackageState_PUBLISHED
	case AptPackageState_AptPackageState_PUBLISHED:
		return next == AptPackageState_AptPackageState_SUPERSEDED
	default:
		return false
	}
}

// EnsureTransitionTo returns an error if the package transition is illegal.
func (s AptPackageState) EnsureTransitionTo(next AptPackageState) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if !s.CanTransitionTo(next) {
		return errors.Wrapf(
			ErrInvalidAptPackageStateTransition,
			"%s -> %s",
			s.String(),
			next.String(),
		)
	}
	return nil
}

// TransitionState transitions the package to the next legal state.
func (p *AptPackage) TransitionState(next AptPackageState) error {
	if err := p.GetState().EnsureTransitionTo(next); err != nil {
		return err
	}
	prev := p.State
	p.State = next
	if err := p.Validate(); err != nil {
		p.State = prev
		return err
	}
	return nil
}
