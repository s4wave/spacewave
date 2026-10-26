package s4wave_alarm

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// AuthenticatedOperation selects the verified transaction signer on replay.
func (o *AcknowledgeAlarmOp) AuthenticatedOperation() {}

// Validate requires an Alarm and a time.
func (o *AcknowledgeAlarmOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if o.GetTimeMs() <= 0 {
		return errors.New("acknowledge time is required")
	}
	return nil
}

// ApplyWorldOp acknowledges the Alarm as the sender.
func (o *AcknowledgeAlarmOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	return false, AcknowledgeAlarm(ctx, ws, o.GetObjectKey(), sender.String(), time.UnixMilli(o.GetTimeMs()))
}

// _ is a type assertion
var _ world.AuthenticatedOperation = (*AcknowledgeAlarmOp)(nil)
