package runtime_controller

import "context"

// rtState contains information about the runtime state.
type rtState struct {
	// synced indicates a sync has been performed
	synced bool
}

// syncOnce queries the frontend if necessary and performs a sync.
func (c *Controller) syncOnce(ctx context.Context) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	st := &c.rtState
	if !st.synced {
		if err := c.queryState(ctx, st); err != nil {
			return err
		}
	}
	return nil
}

// queryState queries the frontend runtime for state.
// called by syncOnce with mtx locked
func (c *Controller) queryState(ctx context.Context, st *rtState) error {
	st.synced = true
	c.le.Info("querying frontend for state")
	return nil
}
