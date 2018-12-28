package reconciler_example

import (
	"context"

	"github.com/aperturerobotics/hydra/reconciler"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Example volume controller.
const ControllerID = "hydra/reconciler/example/1"

// Version is the version of the badger implementation.
var Version = semver.MustParse("0.0.1")

// Reconciler implements a basic example reconciler.
type Reconciler struct {
	// le is the logger
	le *logrus.Entry
}

// NewReconciler is the reconciler constructor.
func NewReconciler(
	le *logrus.Entry,
	conf *Config,
) reconciler.Reconciler {
	return &Reconciler{le: le}
}

// Execute executes the reconciler controller.
func (r *Reconciler) Execute(ctx context.Context, handle reconciler.Handle) error {
	r.le.Info("executing example reconciler")
	// TODO
	return nil
}

// Close releases any resources used by the controller.
func (r *Reconciler) Close() error {
	return nil
}

// _ is a type assertion
var _ reconciler.Reconciler = ((*Reconciler)(nil))
