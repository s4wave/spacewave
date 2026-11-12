package sobject

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// BodyHost runs the BodyDriver of the body a SharedObject carries, for the
// provider that hosts the SharedObject. The provider implements SetBody with
// it. With a signer it also checkpoints as the driver owes, and votes in the
// group's decisions with the driver as the checkpoint judge.
type BodyHost struct {
	// le is the logger.
	le *logrus.Entry
	// so is the hosted SharedObject.
	so SharedObject
	// report receives what the health shows of the replay, or is nil.
	report func(*BodyReport)
	// host holds the state, or is nil for a host that signs nothing.
	host *SOHost
	// privKey signs as the local participant when host is set.
	privKey crypto.PrivKey
	// sfs builds the key epoch transforms when host is set.
	sfs *block_transform.StepFactorySet
	// rc runs the driver of the current body.
	rc *routine.StateRoutineContainer[Body]
	// bcast guards body and wakes the attaches waiting for its release.
	bcast broadcast.Broadcast
	// body is the attached body, or nil.
	body Body
}

// NewBodyHost returns the body host of so. report receives what the health
// shows of the replay, and may be nil.
func NewBodyHost(le *logrus.Entry, so SharedObject, report func(*BodyReport)) *BodyHost {
	h := &BodyHost{le: le, so: so, report: report}
	h.rc = routine.NewStateRoutineContainer[Body](
		func(a, b Body) bool { return a == b },
		routine.WithExitLogger(le.WithField("routine", "body-driver")),
	)
	h.rc.SetStateRoutine(h.execute)
	return h
}

// SetSigner has the host checkpoint and vote as the participant privKey,
// through host. Call it before SetContext.
func (h *BodyHost) SetSigner(host *SOHost, privKey crypto.PrivKey, sfs *block_transform.StepFactorySet) {
	h.host, h.privKey, h.sfs = host, privKey, sfs
}

// SetContext runs the body's driver under ctx, until the next SetContext or
// ClearContext.
func (h *BodyHost) SetContext(ctx context.Context) {
	h.rc.SetContext(ctx, true)
}

// ClearContext stops the driver.
func (h *BodyHost) ClearContext() {
	h.rc.ClearContext()
}

// SetBody waits until no body is attached, then attaches body until release,
// so the SharedObject has one body at a time: a body attaches only once the
// previous one was released. Returns ctx's error if it ends first.
func (h *BodyHost) SetBody(ctx context.Context, body Body) (release func(), err error) {
	// Wait for the previous body's release, then attach it.
	for {
		var attached bool
		var wait <-chan struct{}
		h.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if h.body == nil {
				h.body, attached = body, true
				h.rc.SetState(body)
				return
			}
			wait = getWaitCh()
		})
		if attached {
			break
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-wait:
		}
	}

	// Detach it once and wake the next attach.
	var once sync.Once
	return func() {
		once.Do(func() {
			h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				h.body = nil
				h.rc.SetState(nil)
				broadcast()
			})
		})
	}, nil
}

// execute drives body, and votes with it as the judge when the host signs.
func (h *BodyHost) execute(ctx context.Context, body Body) error {
	// Build the driver.
	d := NewBodyDriver(h.le, h.so, body, h.report)
	if h.host == nil {
		return d.Execute(ctx)
	}
	d.SetCheckpointer(h.host, h.privKey, h.sfs)

	// Vote in the group's decisions beside it.
	control, err := newControl(h.le, h.host, h.privKey, d)
	if err != nil {
		return err
	}
	d.SetVoteWake(control.Wake)
	go func() {
		if err := control.Execute(ctx); err != nil && ctx.Err() == nil {
			h.le.WithError(err).Warn("stopped voting in group decisions")
		}
	}()
	return d.Execute(ctx)
}
