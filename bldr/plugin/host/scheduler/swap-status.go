package plugin_host_scheduler

import (
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// swapCandidate names the manifest a swap state moves to and when the
// scheduler began preparing it.
type swapCandidate struct {
	ref       string
	rev       uint64
	startedAt *timestamp.Timestamp
}

// newSwapCandidate names the manifest args selects, starting now.
func newSwapCandidate(args *executePluginArgs) swapCandidate {
	return swapCandidate{
		ref:       manifestSnapshotRefString(args.manifestSnapshot),
		rev:       args.rev,
		startedAt: timestamp.Now(),
	}
}

// setSwap publishes the logical instance's swap state.
func (t *pluginInstance) setSwap(swap *bldr_plugin.PluginSwap) {
	t.c.updatePluginExecutionStatus(t.pluginID, t.instanceKey, func(status *bldr_plugin.PluginExecutionStatus) {
		status.Swap = swap
	})
}

// setSwapPreparing says the candidate is being fetched and started.
func (t *pluginInstance) setSwapPreparing(c swapCandidate) {
	t.setSwap(&bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Preparing{Preparing: &bldr_plugin.PluginSwapPreparing{
		CandidateManifestRef: c.ref,
		CandidateManifestRev: c.rev,
		StartedAt:            c.startedAt,
	}}})
}

// setSwapWaiting says the candidate waits on the running execution's answer
// to the activation check sent at sinceAt.
func (t *pluginInstance) setSwapWaiting(c swapCandidate, sinceAt *timestamp.Timestamp) {
	t.setSwap(&bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Waiting{Waiting: &bldr_plugin.PluginSwapWaiting{
		CandidateManifestRef: c.ref,
		CandidateManifestRev: c.rev,
		SinceAt:              sinceAt,
	}}})
}

// setSwapFailed says the candidate failed with msg. The execute routine's
// backoff fills RetryAt when it schedules the retry.
func (t *pluginInstance) setSwapFailed(c swapCandidate, msg string) {
	t.setSwap(&bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Failed{Failed: &bldr_plugin.PluginSwapFailed{
		CandidateManifestRef: c.ref,
		CandidateManifestRev: c.rev,
		ErrorMessage:         msg,
		FailedAt:             timestamp.Now(),
	}}})
}

// setSwapSettled says the candidate was admitted at admittedAt.
func (t *pluginInstance) setSwapSettled(admittedAt time.Time) {
	t.setSwap(&bldr_plugin.PluginSwap{State: &bldr_plugin.PluginSwap_Settled{Settled: &bldr_plugin.PluginSwapSettled{
		AdmittedAt: timestamp.New(admittedAt),
	}}})
}

// setSwapRetry records when the execute routine retries a failed candidate.
// The routine draws delay only after the candidate's attempt returned an
// error, so the failed state is already published.
func (t *pluginInstance) setSwapRetry(delay time.Duration) {
	if delay == cbackoff.Stop {
		return
	}
	retryAt := timestamp.New(time.Now().Add(delay))
	t.c.updatePluginExecutionStatus(t.pluginID, t.instanceKey, func(status *bldr_plugin.PluginExecutionStatus) {
		if failed := status.GetSwap().GetFailed(); failed != nil {
			failed.RetryAt = retryAt
		}
	})
}

// setRunningManifest publishes the manifest the admitted execution runs, or
// clears it when args is nil.
func (t *pluginInstance) setRunningManifest(args *executePluginArgs) {
	var ref string
	var rev uint64
	if args != nil {
		ref = manifestSnapshotRefString(args.manifestSnapshot)
		rev = args.rev
	}
	t.c.updatePluginExecutionStatus(t.pluginID, t.instanceKey, func(status *bldr_plugin.PluginExecutionStatus) {
		status.RunningManifestRef = ref
		status.RunningManifestRev = rev
	})
}
