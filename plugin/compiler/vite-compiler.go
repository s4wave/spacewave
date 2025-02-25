//go:build !js

package bldr_plugin_compiler

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/sirupsen/logrus"
)

// viteCompilerTracker is a running Vite compiler instance.
type viteCompilerTracker struct {
	// c is the controller
	c *Controller
	// key is the vite compiler key
	key string
	// le is the logger
	le *logrus.Entry
	// instancePromiseCtr contains the vite compiler rpc instance or any error running it
	instancePromiseCtr *promise.PromiseContainer[*viteCompilerInstance]
}

// buildViteCompilerTracker returns a function that constructs a new Vite compiler tracker.
func (c *Controller) buildViteCompilerTracker(key string) (keyed.Routine, *viteCompilerTracker) {
	le := c.GetLogger().WithField("vite-key", key)
	tr := &viteCompilerTracker{
		c:                  c,
		key:                key,
		le:                 le,
		instancePromiseCtr: promise.NewPromiseContainer[*viteCompilerInstance](),
	}
	return tr.execute, tr
}

// viteCompilerInstance is the vite compiler rpc instance.
type viteCompilerInstance struct {
}

// execute executes the tracker.
func (t *viteCompilerTracker) execute(ctx context.Context) error {
	t.instancePromiseCtr.SetPromise(nil)

	// TODO: Implement Vite compilation logic here
	// This would include:
	// 1. Setting up the Vite environment
	// 2. Running the compilation
	// 3. Handling the output

	// Wait for context cancellation
	<-ctx.Done()
	return context.Canceled
}
