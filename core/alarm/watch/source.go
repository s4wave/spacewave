package core_alarm_watch

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// source is the query resource of the watch's source object. It opens the
// resource on first use, follows its targets, and reopens it after a failure.
type source struct {
	// c is the controller.
	c *Controller
	// changed receives a wake after the targets change, and once retryDelay
	// passes after a failure.
	changed chan struct{}

	// cl is the open client, or nil.
	cl s4wave_prom.SRPCPromQueryResourceServiceClient
	// broken is set when the targets stream of cl ends.
	broken *atomic.Bool
	// close releases cl and ends its targets stream.
	close func()
	// retry stops the pending retry wake, or is nil.
	retry *time.Timer
}

// newSource constructs the source of c.
func newSource(c *Controller) *source {
	return &source{c: c, changed: make(chan struct{}, 1)}
}

// client returns the open client, opening the resource when it is closed or
// its stream ended. A failure schedules a retry wake.
func (s *source) client(ctx context.Context) (s4wave_prom.SRPCPromQueryResourceServiceClient, error) {
	// Reuse a client whose stream still runs.
	if s.cl != nil && !s.broken.Load() {
		return s.cl, nil
	}
	s.release()

	// Open the resource.
	cl, closeFn, broken, err := s.open(ctx)
	if err != nil {
		s.fail()
		return nil, err
	}
	s.cl, s.close, s.broken = cl, closeFn, broken
	return cl, nil
}

// fail closes the client after a failed call and schedules a retry wake.
func (s *source) fail() {
	s.release()
	s.retry = time.AfterFunc(retryDelay, s.wake)
}

// wake signals changed without blocking.
func (s *source) wake() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// release closes the client and stops the pending retry.
func (s *source) release() {
	if s.retry != nil {
		s.retry.Stop()
		s.retry = nil
	}
	if s.close != nil {
		s.close()
	}
	s.cl, s.close, s.broken = nil, nil, nil
}

// open builds the source object's resource from its registered object type
// and follows its targets until the returned close function is called.
func (s *source) open(ctx context.Context) (s4wave_prom.SRPCPromQueryResourceServiceClient, func(), *atomic.Bool, error) {
	// Read the source's object type.
	key := s.c.conf.GetSourceKey()
	ws := world.NewEngineWorldState(s.c.engine, false)
	typeID, err := world_types.GetObjectType(ctx, ws, key)
	if err != nil {
		return nil, nil, nil, err
	}
	if typeID == "" {
		return nil, nil, nil, errors.Errorf("source %q has no object type", key)
	}

	// Resolve the object type under the watch's World engine scope.
	ctx = objecttype.WithEngineID(ctx, s.c.conf.GetEngineId())
	objType, ref, err := objecttype.ExLookupObjectType(ctx, s.c.b, typeID)
	if err != nil {
		return nil, nil, nil, err
	}
	if objType == nil {
		return nil, nil, nil, errors.Errorf("object type %q is not registered", typeID)
	}

	// Construct the object's resource with the type's factory.
	invoker, cleanup, err := objType.GetFactory()(ctx, s.c.le, s.c.b, s.c.engine, ws, key)
	if err != nil {
		ref.Release()
		return nil, nil, nil, errors.Wrap(err, "object type factory")
	}
	streamCtx, cancel := context.WithCancel(ctx)
	closeFn := func() {
		cancel()
		if cleanup != nil {
			cleanup()
		}
		ref.Release()
	}
	if invoker == nil {
		closeFn()
		return nil, nil, nil, errors.Errorf("object type %q serves no resource", typeID)
	}

	// Call the resource in process, and wake the watch after each change to
	// its targets.
	cl := s4wave_prom.NewSRPCPromQueryResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker))))
	broken := &atomic.Bool{}
	go s.follow(streamCtx, cl, broken)
	return cl, closeFn, broken, nil
}

// follow wakes the watch after each change to the targets of cl. When the
// stream ends before ctx, it marks the client broken and wakes the watch after
// retryDelay.
func (s *source) follow(ctx context.Context, cl s4wave_prom.SRPCPromQueryResourceServiceClient, broken *atomic.Bool) {
	// Open the stream.
	stream, err := cl.WatchTargets(ctx, &s4wave_prom.WatchTargetsRequest{})
	if err == nil {
		defer stream.Close()
	}

	// Wake on each change until the stream ends.
	for err == nil {
		if _, err = stream.Recv(); err == nil {
			s.wake()
		}
	}
	if ctx.Err() != nil {
		return
	}
	s.c.le.WithError(err).Warn("alarm watch source stream ended")
	broken.Store(true)
	time.AfterFunc(retryDelay, s.wake)
}
