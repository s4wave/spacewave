package s4wave_wizard

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// WizardTypePrefix is the prefix for all wizard object type IDs.
const WizardTypePrefix = "wizard/"

// WizardFactory creates a WizardResource from a world object.
func WizardFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	var state *WizardState
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, world.ErrObjectNotFound
	}

	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = UnmarshalWizardState(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, nil, err
	}

	if state == nil {
		state = &WizardState{}
	}

	resource := NewWizardResource(ws, engine, objectKey, state)
	return resource.GetMux(), resource.Close, nil
}

// LookupWizardObjectType looks up an ObjectType for wizard/* type IDs.
// Returns nil if the type ID does not have the wizard/ prefix.
func LookupWizardObjectType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	if !strings.HasPrefix(typeID, WizardTypePrefix) {
		return nil, nil
	}
	return objecttype.NewObjectType(typeID, WizardFactory), nil
}
