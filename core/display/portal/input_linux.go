package portal

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/s4wave/spacewave/core/frametrack"
)

// inputCode identifies an evdev key or a mouse button.
type inputCode struct {
	// code is the Linux input event code.
	code int32
	// button selects the portal's pointer method for this code.
	button bool
}

// Input owns the keys and buttons one admitted viewer holds.
type Input struct {
	// session serializes this viewer with the other viewers of the same screen.
	session *Session
	// held records this viewer's presses under the session's input lock.
	held map[inputCode]struct{}
}

// NewInput binds a viewer to the session's pointer and keyboard.
func (s *Session) NewInput() *Input {
	return &Input{session: s, held: make(map[inputCode]struct{})}
}

// Send applies a validated event in the order it arrived on the control stream.
func (i *Input) Send(ctx context.Context, event *frametrack.Input) error {
	// Keep a motion and its button transitions together across viewers.
	s := i.session
	unlock, err := s.inputMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	// Place the cursor in the stream's logical monitor before changing buttons.
	// Finish an admitted event on the portal lifetime so disconnect can release it.
	if pointer := event.GetPointer(); pointer != nil {
		x := math.Min(float64(s.width-1), float64(pointer.GetX())*float64(s.width))
		y := math.Min(float64(s.height-1), float64(pointer.GetY())*float64(s.height))
		if err := s.notify("NotifyPointerMotionAbsolute", s.nodeID, x, y); err != nil {
			return err
		}
		for bit, code := range pointerButtons {
			if err := i.setPressed(inputCode{code: code, button: true}, pointer.GetButtons()&(1<<bit) != 0); err != nil {
				return err
			}
		}
		return nil
	}

	// Forward physical keys and let the compositor own auto-repeat.
	if key := event.GetKey(); key != nil && !key.GetRepeat() {
		if code := evdevKeycodes[key.GetCode()]; code != 0 {
			return i.setPressed(inputCode{code: code}, key.GetPressed())
		}
	}
	return nil
}

// Close releases the presses held only by this viewer while the portal lives.
func (i *Input) Close() error {
	// Use the portal lifetime because the viewer's stream can already be closed.
	ctx := i.session.Context()
	unlock, err := i.session.inputMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	// Release every held code, even if the portal reports a failure on one.
	for code := range i.held {
		err = errors.Join(err, i.setPressed(code, false))
	}
	if err != nil && ctx.Err() == nil {
		i.session.cancel(err)
	}
	return err
}

// setPressed changes one viewer's vote under the session's input lock.
func (i *Input) setPressed(code inputCode, pressed bool) error {
	// Duplicates and other viewers' held presses need no bus call.
	_, held := i.held[code]
	if held == pressed {
		return nil
	}
	count := i.session.pressed[code]
	if pressed && count == 0 {
		if err := i.notifyPress(code, 1); err != nil {
			return err
		}
	}
	if !pressed && count == 1 {
		if err := i.notifyPress(code, 0); err != nil {
			return err
		}
	}

	// Commit the viewer's vote after the compositor accepts the transition.
	switch {
	case pressed:
		i.held[code] = struct{}{}
		i.session.pressed[code] = count + 1
	case count == 1:
		delete(i.held, code)
		delete(i.session.pressed, code)
	default:
		delete(i.held, code)
		i.session.pressed[code] = count - 1
	}
	return nil
}

// notifyPress sends an evdev button or key transition.
func (i *Input) notifyPress(code inputCode, pressed uint32) error {
	method := "NotifyKeyboardKeycode"
	if code.button {
		method = "NotifyPointerButton"
	}
	return i.session.notify(method, code.code, pressed)
}

// inputWriteLimit bounds backpressure from the session bus socket.
const inputWriteLimit = time.Second

// notify queues input on the session connection while inputMtx is held.
func (s *Session) notify(method string, params ...any) error {
	// A revoked session cannot send another event.
	if cause := context.Cause(s.ctx); cause != nil {
		return cause
	}

	// Notify methods have no reply payload. Bound only the local socket write.
	deadline := time.Now().Add(inputWriteLimit)
	err := s.socket.SetWriteDeadline(deadline)
	if err == nil {
		err = s.conn.Object(s.owner, desktopPath).Call(remoteDesktop+"."+method, dbus.FlagNoReplyExpected,
			append([]any{s.path, map[string]dbus.Variant{}}, params...)...).Err
	}

	// Godbus does not report socket write errors for no-reply calls. A write
	// that reaches the deadline may be partial, so retire the connection.
	if !time.Now().Before(deadline) {
		err = errors.Join(err, context.DeadlineExceeded)
	}
	if err == nil {
		err = s.socket.SetWriteDeadline(time.Time{})
	}
	if err != nil {
		s.cancel(err)
		_ = s.socket.Close()
	}
	return err
}
