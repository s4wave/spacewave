package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/sobject"
)

// waitAccountTransition follows the canonical signed redirect under this
// unlocked Session's lifetime. Provider retry retains recoverable source data.
func (s *Session) waitAccountTransition(ctx context.Context) error {
	// Follow the settings binding through a retrying state routine.
	a := s.tkr.a
	watcher := routine.NewStateRoutineContainerWithLoggerVT[*sobject.SharedObjectRef](a.le.WithField("routine", "account-transition"), routine.WithRetry(providerBackoff))
	watcher.SetStateRoutine(s.followAccountTransition)
	watcher.SetContext(ctx, false)
	defer func() {
		if exited, _, _ := watcher.SetStateRoutine(nil); exited != nil {
			<-exited
		}
	}()

	// Enrollment replaces a fresh account's empty settings binding and publishes
	// the catalog afterward. Follow that owner notification before mounting state.
	var previous *sobject.SharedObjectList
	for {
		var err error
		previous, err = a.soListCtr.WaitValueChange(ctx, previous, nil)
		if err != nil {
			return err
		}
		ref, err := a.GetAccountSettingsRef(ctx)
		if err != nil {
			return err
		}
		watcher.SetState(ref)
	}
}

func (s *Session) followAccountTransition(ctx context.Context, ref *sobject.SharedObjectRef) error {
	// Follow each settings state change until the Session is reattached.
	a := s.tkr.a
	if ref == nil {
		return nil
	}

	// Mount the settings object and watch its state for changes.
	object, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer release()
	states, releaseStates, err := object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer releaseStates()

	// Decode each settings snapshot and validate its redirect.
	var previous sobject.SharedObjectStateSnapshot
	for {
		previous, err = states.WaitValueChange(ctx, previous, nil)
		if err != nil {
			return err
		}
		if previous == nil {
			continue
		}
		settings, _, err := decodeAccountSettingsSnapshot(ctx, previous)
		if err != nil {
			return err
		}
		transition := settings.GetTransition()
		if transition == nil {
			continue
		}
		if err := transition.Validate(); err != nil {
			return err
		}
		if !transition.GetSource().EqualVT(ref.GetProviderResourceRef()) {
			return errors.New("account settings redirect belongs to another source")
		}

		// The active pairing exchange owns this client's commit and final receipt.
		// A returning or separately mounted Session follows the persisted result.
		s.pairingMu.Lock()
		engine := s.pairingEngine
		s.pairingMu.Unlock()
		if engine != nil {
			for {
				snapshot, changed := engine.Snapshot()
				if snapshot.Status == pairing.StatusBothConfirmed && snapshot.Choice.Merging() {
					// The pairing owner already committed this attachment.
					<-ctx.Done()
					return ctx.Err()
				}
				if snapshot.Status != pairing.StatusEnrolling || !snapshot.Choice.Merging() {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-changed:
				}
			}
		}

		// Resolve the destination provider and its account.
		p, releaseProvider, err := provider.ExLookupProvider(ctx, a.t.p.b, transition.GetDestination().GetProviderId(), false, nil)
		if err != nil {
			return err
		}
		defer releaseProvider.Release()
		if p == nil {
			return errors.New("destination account provider is not configured")
		}
		account, releaseAccount, err := p.AccessProviderAccount(ctx, transition.GetDestination().GetProviderAccountId(), nil)
		if err != nil {
			return err
		}
		defer releaseAccount()
		target, ok := account.(provider_migration.Account)
		if !ok {
			return errors.New("destination provider cannot accept a returning Session")
		}

		// Attach this Session to the destination and rebind its provider.
		next, err := target.AttachMigratedSession(ctx, s, transition)
		if err != nil {
			return err
		}
		if err := provider_migration.RebindSession(ctx, s, next); err != nil {
			return err
		}
		<-ctx.Done()
		return ctx.Err()
	}
}
