package web_document_controller

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/document/view"
)

// cState contains information about the controller state.
type cState struct {
	// synced indicates a sync has been performed
	synced bool
	// webViews is the most recent set of web views
	webViews map[string]web_view.WebView
}

// syncOnce queries the frontend if necessary and performs a sync.
func (c *Controller) syncOnce(ctx context.Context) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	st := &c.cState
	if !st.synced {
		if err := c.queryState(ctx, st); err != nil {
			return err
		}
	}
	return nil
}

// queryState queries the frontend runtime for state.
// called by syncOnce with mtx locked
func (c *Controller) queryState(ctx context.Context, st *cState) error {
	wv, err := c.rt.GetWebViews(ctx)
	if err != nil {
		return err
	}

	st.webViews = wv
	st.synced = true
	return nil
}
