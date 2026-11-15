package sobject

import (
	"context"
	"strings"

	"github.com/pkg/errors"
)

// parityProbeErrClass names the kind of err for the reload parity probe's log
// lines, which carry no error text: the text may name refs and keys.
func parityProbeErrClass(err error) string {
	msg := err.Error()
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case strings.Contains(msg, "waits for a handler"):
		return "op-handler-pending"
	case strings.Contains(msg, "not available"), strings.Contains(msg, "not found"):
		return "not-found"
	case strings.Contains(msg, "stage"):
		return "stage"
	case strings.Contains(msg, "decrypt"), strings.Contains(msg, "transform"):
		return "transform"
	case strings.Contains(msg, "checkpoint"):
		return "checkpoint"
	default:
		return "other"
	}
}

// parityProbeWaitState names the state of the replay wait err carries, or none
// when err is not a wait.
func parityProbeWaitState(err error) string {
	var w *BodyWait
	if !errors.As(err, &w) {
		return "none"
	}
	switch w.Wait.GetState().(type) {
	case *SOReplayWait_Handler:
		return "handler"
	case *SOReplayWait_Approval:
		return "approval"
	default:
		return "unset"
	}
}
