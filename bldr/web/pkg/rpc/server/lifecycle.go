package web_pkg_rpc_server

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/keyed"
)

func (c *Controller) addWebPkgRef(key string) (*keyed.KeyedRef[string, *webPkgTracker], *webPkgTracker, error) {
	c.lifecycleMtx.Lock()
	defer c.lifecycleMtx.Unlock()
	if c.closed {
		return nil, nil, context.Canceled
	}
	ref, tracker, _ := c.webPkgs.AddKeyRef(key)
	return ref, tracker, nil
}

func (c *Controller) releaseWebPkgRef(ref *keyed.KeyedRef[string, *webPkgTracker]) {
	c.lifecycleMtx.Lock()
	if c.closed || c.releaseDelay == 0 {
		c.lifecycleMtx.Unlock()
		ref.Release()
		return
	}

	c.delayedWG.Add(1)
	timer := time.AfterFunc(c.releaseDelay, func() {
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
	c.lifecycleMtx.Lock()
	refs := make([]*keyed.KeyedRef[string, *webPkgTracker], 0, len(c.delayedReleases))
	for ref, timer := range c.delayedReleases {
		_ = timer.Stop()
		refs = append(refs, ref)
	}
	clear(c.delayedReleases)
	c.lifecycleMtx.Unlock()

	for _, ref := range refs {
		ref.Release()
	}
}
