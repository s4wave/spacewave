package spacewave_launcher_controller

import (
	"context"
	"io"
	"math/rand/v2"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
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

	// Publish the fetch in flight, counting it as another attempt.
	attempts := currLauncherInfo.GetFetchStatus().GetAttempts() + 1
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.Fetching = true
		next.HasConfig = currRev != 0
		next.Attempts = attempts
		next.NextRetryAt = nil
	})
	var fetchedRev uint64
	var fetchedSource string
	defer func() {
		info := c.launcherInfoCtr.GetValue()
		c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
			next.Fetching = false
			next.HasConfig = info.GetDistConfig().GetRev() != 0
			next.SelectedConfigRev = info.GetDistConfig().GetRev()
			if fetchedRev != 0 {
				next.FetchedConfigRev = fetchedRev
				next.FetchedConfigSource = fetchedSource
			}
			next.LastError = ""
			next.Attempts = 0
			if rerr != nil {
				next.LastError = rerr.Error()
				next.Attempts = attempts
			}
		})
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
		fetchedRev = rev
		fetchedSource = endpURLStr
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
		c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
			next.SelectedConfigRev = rev
			next.SelectedConfigSource = spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_ENDPOINT
			next.FetchedConfigRev = fetchedRev
			next.FetchedConfigSource = fetchedSource
		})
		c.RecheckReleaseMetadata()
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
	// Stamp the next fetch time so fetch status watchers can render a
	// countdown without reimplementing the backoff.
	nextRetryAt := timestamp.ToTimestamp(time.Now().Add(refetchDur))
	c.updateFetchStatus(func(next *spacewave_launcher.FetchStatus) {
		next.NextRetryAt = nextRetryAt
	})
	c.confFetcherRefetch = time.AfterFunc(refetchDur, func() {
		_ = c.confFetcherRoutine.RestartRoutine()
	})
}

// _ is a type assertion
var _ routine.Routine = (*Controller)(nil).fetchDistConfig
