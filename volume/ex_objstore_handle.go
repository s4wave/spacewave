package volume

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// BuildObjectStoreAPIEx executes building an object store api.
func BuildObjectStoreAPIEx(
	ctx context.Context,
	b bus.Bus,
	storeID, storeVolume string,
) (BuildObjectStoreAPIValue, directive.Reference, error) {
	// Acquire handle to storage.
	osv, osRef, err := bus.ExecOneOff(
		ctx,
		b,
		NewBuildObjectStoreAPI(
			storeID, storeVolume,
		),
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	objs, ok := osv.GetValue().(BuildObjectStoreAPIValue)
	if !ok {
		osRef.Release()
		return nil, nil, errors.New("build object store api value invalid")
	}
	if err := objs.GetError(); err != nil {
		osRef.Release()
		return nil, nil, err
	}
	return objs, osRef, nil
}
