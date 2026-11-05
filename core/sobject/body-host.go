package sobject

import (
	"context"
	"sync"

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
	// mtx orders attaching and detaching bodies.
	mtx sync.Mutex
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

// SetBody attaches body, replacing the attached body, until release, which
// detaches it only while it is still attached. The driver of the previous
// body stops first.
func (h *BodyHost) SetBody(body Body) (release func()) {
	// Attach it.
	h.mtx.Lock()
	h.body = body
	h.rc.SetState(body)
	h.mtx.Unlock()

	// Detach it once, unless another body replaced it.
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mtx.Lock()
			defer h.mtx.Unlock()
			if h.body == body {
				h.body = nil
				h.rc.SetState(nil)
			}
		})
	}
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
	go func() {
		if err := control.Execute(ctx); err != nil && ctx.Err() == nil {
			h.le.WithError(err).Warn("stopped voting in group decisions")
		}
	}()
	return d.Execute(ctx)
}
