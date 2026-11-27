//go:build !js && !bldr_sqlite

package devtool

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// reopenClaimHook releases a retained database when another open reaches its claim.
type reopenClaimHook struct {
	// claim transfers the database to the first conflicting open diagnostic.
	claim chan *s4db.DB
	// closed reports the result of releasing that database.
	closed chan error
}

// Levels observes construction warnings and loader failures.
func (h *reopenClaimHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire releases the claim only after construction encounters the conflicting open.
func (h *reopenClaimHook) Fire(entry *logrus.Entry) error {
	// Match both controller errors and loader diagnostics.
	err, _ := entry.Data[logrus.ErrorKey].(error)
	if !errors.Is(err, s4db.ErrOpenInProcess) &&
		!strings.Contains(entry.Message, s4db.ErrOpenInProcess.Error()) {
		return nil
	}

	// Transfer the claim once even when concurrent diagnostics report the failure.
	select {
	case db := <-h.claim:
		h.closed <- db.Close()
	default:
	}
	return nil
}

// _ is a type assertion
var _ logrus.Hook = (*reopenClaimHook)(nil)
