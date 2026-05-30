package cdn_sharedobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/sirupsen/logrus"
)

// ResolveMountSharedObjectBody builds the resolver for a CDN-backed Space body.
func ResolveMountSharedObjectBody(
	le *logrus.Entry,
	b bus.Bus,
	dir sobject.MountSharedObjectBody,
) ([]directive.Resolver, error) {
	return directive.R(directive.NewAccessResolver(func(ctx context.Context, released func()) (space.MountSharedObjectBodyValue, func(), error) {
		cdnSO, ok := dir.MountSharedObjectBodySource().(*CdnSharedObject)
		if !ok {
			return nil, nil, errors.Errorf("cdn body type on non-cdn shared object: %T", dir.MountSharedObjectBodySource())
		}

		we, err := NewWorldEngine(ctx, le, b, cdnSO, space_world_optypes.LookupWorldOp)
		if err != nil {
			return nil, nil, errors.Wrap(err, "build cdn world engine")
		}
		var body space.SpaceSharedObjectBody = NewCdnSpaceBody(cdnSO, we)
		ret := sobject.NewMountSharedObjectBodyValue[space.SpaceSharedObjectBody](
			dir.MountSharedObjectBodyRef(),
			CdnBodyType,
			cdnSO,
			body,
		)

		return ret, we.Release, nil
	}), nil)
}
