package s4wave_wizard

import (
	"context"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// WizardEndingsObjectKey is the key of the one object per Space that holds
// every wizard ending as an outgoing quad.
const WizardEndingsObjectKey = "wizard-endings"

// WizardEndingPredicatePrefix prefixes the predicate of a wizard ending quad;
// the wizard's key follows it.
const WizardEndingPredicatePrefix = "wizard/ended/"

// WizardEndingPredicate returns the predicate IRI of the ending quad for the
// wizard at wizardKey, such as <wizard/ended/wizard/vm-1>.
func WizardEndingPredicate(wizardKey string) string {
	return quad.IRI(WizardEndingPredicatePrefix + wizardKey).String()
}

// LookupWizardEnding returns the key of the object the wizard at wizardKey
// finished as, and false when the World records no ending for that key: the
// wizard exists, was deleted without a Finish, or the object it finished as
// was deleted since. The lookup is indexed by subject and predicate.
func LookupWizardEnding(ctx context.Context, ws world.WorldStateGraph, wizardKey string) (string, bool, error) {
	return world.LookupGraphObjectKey(ctx, ws, WizardEndingsObjectKey, WizardEndingPredicate(wizardKey))
}

// clearWizardEnding deletes every ending recorded for wizardKey. A missing
// endings object has none.
func clearWizardEnding(ctx context.Context, ws world.WorldState, wizardKey string) error {
	// Find the ending quads for the key.
	filter := world.NewGraphQuadWithKeys(WizardEndingsObjectKey, WizardEndingPredicate(wizardKey), "", "")
	quads, err := ws.LookupGraphQuads(ctx, filter, 0)
	if err != nil {
		return err
	}

	// Delete each one.
	for _, q := range quads {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// ensureWizardEndings creates the typed endings object when it is absent.
func ensureWizardEndings(ctx context.Context, ws world.WorldState) error {
	exists, err := ws.HasObject(ctx, WizardEndingsObjectKey)
	if err != nil {
		return err
	}
	if !exists {
		obj, err := ws.CreateObject(ctx, WizardEndingsObjectKey, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
	}
	return world_types.SetObjectType(ctx, ws, WizardEndingsObjectKey, WizardEndingsTypeID)
}

// Validate performs cursory checks on the op.
func (o *RecordWizardEndingOp) Validate() error {
	if len(o.GetWizardKey()) == 0 {
		return errors.Wrap(world.ErrEmptyObjectKey, "wizard key")
	}
	return nil
}

// ApplyWorldOp records the wizard's ending: <wizard-endings>
// <wizard/ended/KEY> <target>, replacing any ending recorded for KEY.
//
// The ending lives exactly as long as both of these hold:
//   - its target exists: deleting the target, on any path, deletes every quad
//     naming it, this one included;
//   - no wizard was created at KEY since: CreateWizardObjectOp clears the
//     ending, so a reused key, such as wizard/git-repo-1, never reads as
//     finished by an earlier wizard.
//
// Wizard keys are both reused (numbered keys picked as the first free one) and
// unique (timestamped quickstart and VM keys), so endings are quads on one
// object per Space rather than an object per Finish, which nothing would
// delete. A target that does not exist records no ending, rather than failing
// the Finish over its bookkeeping; such a wizard reads as deleted.
func (o *RecordWizardEndingOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Check the op.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Drop any earlier ending for the key.
	if err := clearWizardEnding(ctx, ws, o.GetWizardKey()); err != nil {
		return false, err
	}

	// Record nothing for a target that does not exist.
	targetKey := o.GetTargetKey()
	if targetKey == "" {
		return false, nil
	}
	exists, err := ws.HasObject(ctx, targetKey)
	if err != nil || !exists {
		return false, err
	}

	// Set the ending quad on the endings object.
	if err := ensureWizardEndings(ctx, ws); err != nil {
		return false, err
	}
	ending := world.NewGraphQuadWithKeys(WizardEndingsObjectKey, WizardEndingPredicate(o.GetWizardKey()), targetKey, "")
	return false, ws.SetGraphQuad(ctx, ending)
}

var _ world.Operation = (*RecordWizardEndingOp)(nil)
