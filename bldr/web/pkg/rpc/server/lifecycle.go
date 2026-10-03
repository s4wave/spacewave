package web_pkg_rpc_server

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/keyed"
)

func (c *Controller) addWebPkgRef(key string) (*keyed.KeyedRef[string, *webPkgTracker], *webPkgTracker, error) {
	// Acquire a web package reference while the controller accepts requests.
	c.lifecycleMtx.Lock()
	defer c.lifecycleMtx.Unlock()
	if c.closed {
		return nil, nil, context.Canceled
	}
	ref, tracker, _ := c.webPkgs.AddKeyRef(key)
	return ref, tracker, nil
}

func (c *Controller) releaseWebPkgRef(ref *keyed.KeyedRef[string, *webPkgTracker]) {
	// Release the web package immediately when retention is disabled or shutdown has begun.
	c.lifecycleMtx.Lock()
	if c.closed || c.releaseDelay == 0 {
		c.lifecycleMtx.Unlock()
		ref.Release()
		return
	}

	// Retain the web package reference until its configured release deadline.
	c.delayedWG.Add(1)
	timer := time.AfterFunc(c.releaseDelay, func() {
		// Release the retained package reference and complete its pending timer work.
		defer c.delayedWG.Done()
		c.lifecycleMtx.Lock()
		delete(c.delayedReleases, ref)
		c.lifecycleMtx.Unlock()
		ref.Release()
	})
	c.delayedReleases[ref] = timer
	c.lifecycleMtx.Unlock()
}

func (c *Controller) stopDelayedReleases() {
	// Cancel pending release timers and collect their web package references.
	c.lifecycleMtx.Lock()
	refs := make([]*keyed.KeyedRef[string, *webPkgTracker], 0, len(c.delayedReleases))
	for ref, timer := range c.delayedReleases {
		if timer.Stop() {
			c.delayedWG.Done()
		}
		refs = append(refs, ref)
	}
	clear(c.delayedReleases)
	c.lifecycleMtx.Unlock()

	// Release the references detached from the controller retention map.
	for _, ref := range refs {
		ref.Release()
	}
}
