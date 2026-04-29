package spacewave_launcher_controller

import (
	"context"
	"io"
	"math/rand/v2"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/http"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// defaultFetcherBackoffConf builds the default fetcher backoff config.
func defaultFetcherBackoffConf() *backoff.Backoff {
	return &backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			InitialInterval:     5000,
			MaxInterval:         1000 * 60 * 10, // 10 minutes
			RandomizationFactor: 0.15,
		},
	}
}

// fetchDistConfig is a routine to fetch the dist config from the endpoints.
//
// periodically retries.
func (c *Controller) fetchDistConfig(ctx context.Context) (rerr error) {
	currLauncherInfo, err := c.launcherInfoCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	currDistConf := currLauncherInfo.GetDistConfig()
	currRev := currDistConf.GetRev()

	// publish fetching=true, preserving has-config / attempts state.
	prevStatus := c.fetchStatusCtr.GetValue()
	var attempts uint32
	if prevStatus != nil {
		attempts = prevStatus.Attempts
	}
	attempts++
	c.fetchStatusCtr.SetValue(&spacewave_launcher.FetchStatus{
		Fetching:  true,
		HasConfig: currRev != 0,
		Attempts:  attempts,
	})
	defer func() {
		info := c.launcherInfoCtr.GetValue()
		finalStatus := &spacewave_launcher.FetchStatus{
			HasConfig: info.GetDistConfig().GetRev() != 0,
		}
		if rerr != nil {
			finalStatus.LastErr = rerr.Error()
			finalStatus.Attempts = attempts
		}
		c.fetchStatusCtr.SetValue(finalStatus)
	}()

	var failErr error
	setFailErr := func(err error) {
		if err != nil && failErr == nil {
			failErr = err
		}
	}
	for i, endp := range c.endps {
		endpURLStr := endp.GetUrl()
		c.le.Debugf("calling endpoint %d/%d: %s", i+1, len(c.endps), endpURLStr)
		req, err := http.NewRequestWithContext(ctx, "GET", endpURLStr, nil)
		if err != nil {
			c.le.WithError(err).Warn("skipping invalid endpoint")
			setFailErr(err)
			continue
		}
		for k, v := range endp.GetHeaders() {
			req.Header.Set(k, v)
		}
		resp, err := http.DoRequest(c.le, http.DefaultClient, req, true)
		var dat []byte
		if resp != nil && resp.Body != nil {
			if err == nil {
				dat, err = io.ReadAll(resp.Body)
			}
			_ = resp.Body.Close()
		}
		if err != nil {
			c.le.WithError(err).Warn("failed to fetch endpoint")
			setFailErr(err)
			continue
		}
		updatedAppDistConf, updatedAppDistConfMsg, updatedAppDistConfPeer, err := spacewave_launcher.ParseDistConfigPackedMsg(
			c.le.WithField("endpoint", endpURLStr),
			dat,
			c.distPeerIDs,
			c.conf.GetProjectId(),
		)
		rev := updatedAppDistConf.GetRev()
		if err == nil && rev == 0 {
			err = errors.New("failed to find a valid dist config")
		}
		if err != nil {
			c.le.WithError(err).Warn("skipping endpoint response")
			setFailErr(err)
			continue
		}
		// config is valid: check if newer
		if rev == currRev {
			c.le.Debugf("found valid config with rev equal to current: %d", rev)
			// stop here
			return nil
		}
		if rev < currRev {
			c.le.Debugf("found valid config with rev older than current: %d < %d", rev, currRev)
			// continue searching
			continue
		}

		// config is newer, store it & update
		if err := c.storeDistConf(ctx, []byte(updatedAppDistConfMsg)); err != nil {
			c.le.WithError(err).Warn("failed to store updated app dist config")
		}
		_, _ = c.swapDistConf(updatedAppDistConf)
		c.le.
			WithField("prev-conf-rev", currRev).
			WithField("conf-rev", rev).
			WithField("conf-signer", updatedAppDistConfPeer.String()).
			WithField("conf-channel-key", updatedAppDistConf.ResolvedChannelKey()).
			WithField("endpoint", endpURLStr).
			Info("updated app dist config")
		return nil
	}

	// didn't update anything
	return failErr
}

// confFetcherExited is called when fetchDistConfig exits.
func (c *Controller) confFetcherExited(err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.confFetcherRefetch != nil {
		_ = c.confFetcherRefetch.Stop()
		c.confFetcherRefetch = nil
	}
	if err != nil {
		return
	}

	// schedule retry
	refetchDur, _ := c.conf.ParseRefetchDur()
	if refetchDur <= 0 {
		return
	}
	staggerMs := 0.1 * float32(refetchDur.Milliseconds()) * (rand.Float32()*2.0 - 1.0) //nolint:gosec
	refetchDur += time.Millisecond * time.Duration(staggerMs)

	c.le.Debugf("scheduling re-check in %v", refetchDur.String())
	// stamp the next-retry time so WatchLauncherFetchStatus consumers can
	// render a countdown without having to re-implement backoff themselves.
	if curr := c.fetchStatusCtr.GetValue(); curr != nil {
		next := *curr
		next.NextRetryAt = time.Now().Add(refetchDur)
		c.fetchStatusCtr.SetValue(&next)
	}
	c.confFetcherRefetch = time.AfterFunc(refetchDur, func() {
		_ = c.confFetcherRoutine.RestartRoutine()
	})
}

// _ is a type assertion
var _ routine.Routine = ((*Controller)(nil)).fetchDistConfig
