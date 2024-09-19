package volume

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// ExBuildObjectStoreAPI executes building the object store api.
func ExBuildObjectStoreAPI(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	storeID, storeVolume string,
	disposeCb func(),
) (BuildObjectStoreAPIValue, directive.Instance, directive.Reference, error) {
	// Acquire handle to storage.
	objs, di, osRef, err := bus.ExecWaitValue[BuildObjectStoreAPIValue](
		ctx,
		b,
		NewBuildObjectStoreAPI(
			storeID, storeVolume,
		),
		bus.ReturnIfIdle(returnIfIdle),
		disposeCb,
		nil,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	return objs, di, osRef, nil
}
