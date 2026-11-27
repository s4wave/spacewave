package plugin_host_scheduler

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// PluginStatus returns the current plugin status snapshot.
func (c *Controller) PluginStatus() *bldr_plugin.PluginStatusSnapshot {
	return c.pluginStatus.GetValue()
}

// AwaitPluginStatus waits for a snapshot accepted by valid.
func (c *Controller) AwaitPluginStatus(
	ctx context.Context,
	valid func(*bldr_plugin.PluginStatusSnapshot) (bool, error),
) (*bldr_plugin.PluginStatusSnapshot, error) {
	return c.pluginStatus.WaitValueWithValidator(ctx, valid, nil)
}

// WaitPluginStatusChange waits until the scheduler's status differs from previous.
func (c *Controller) WaitPluginStatusChange(
	ctx context.Context,
	previous *bldr_plugin.PluginStatusSnapshot,
) (*bldr_plugin.PluginStatusSnapshot, error) {
	return c.pluginStatus.WaitValueChange(ctx, previous, nil)
}

// GetInstanceKey returns the instance key the scheduler was configured with.
func (c *Controller) GetInstanceKey() string {
	return c.conf.GetInstanceKey()
}

// FindControllerOnBus returns the first plugin host scheduler on b.
func FindControllerOnBus(b bus.Bus) *Controller {
	for _, ctrl := range b.GetControllers() {
		scheduler, ok := ctrl.(*Controller)
		if ok {
			return scheduler
		}
	}
	return nil
}

// WaitControllerOnBus waits for a plugin host scheduler to be added to b.
func WaitControllerOnBus(ctx context.Context, b bus.Bus) (*Controller, error) {
	var scheduler *Controller
	err := b.GetControllersBroadcast().Wait(ctx, func(
		_ func(),
		_ func() <-chan struct{},
	) (bool, error) {
		scheduler = FindControllerOnBus(b)
		return scheduler != nil, nil
	})
	if err != nil {
		return nil, err
	}
	return scheduler, nil
}

// IsPluginRunningOnBus reports whether the scheduler on b has a running plugin.
func IsPluginRunningOnBus(ctx context.Context, b bus.Bus, pluginID string) bool {
	if err := ctx.Err(); err != nil {
		return false
	}
	scheduler := FindControllerOnBus(b)
	return scheduler != nil && scheduler.IsPluginRunning(pluginID)
}

// IsPluginRunning reports whether any instance for pluginID is running.
func (c *Controller) IsPluginRunning(pluginID string) bool {
	for _, plugin := range c.pluginStatus.GetValue().GetPlugins() {
		if plugin.GetPluginId() == pluginID && plugin.GetRunning() {
			return true
		}
	}
	return false
}

// WaitPluginsRunning waits until all shared instances in pluginIDs report
// running, or returns the first scheduler error recorded for one of them.
func (c *Controller) WaitPluginsRunning(ctx context.Context, pluginIDs []string) error {
	// Return immediately with no plugin IDs.
	if len(pluginIDs) == 0 {
		return nil
	}

	// Collect the non-empty required plugin IDs.
	required := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		if pluginID != "" {
			required[pluginID] = struct{}{}
		}
	}
	if len(required) == 0 {
		return nil
	}

	// Wait for each status snapshot until every required plugin runs or one
	// reports an error.
	var done error
	_, err := c.pluginStatus.WaitValueWithValidator(ctx, func(snapshot *bldr_plugin.PluginStatusSnapshot) (bool, error) {
		running, err := pluginsRunningOrError(snapshot, required)
		done = err
		return running || err != nil, nil
	}, nil)
	if err != nil {
		return err
	}
	return done
}

// pluginsRunningOrError reports whether every required plugin is running,
// returning an error naming any missing one.
func pluginsRunningOrError(snapshot *bldr_plugin.PluginStatusSnapshot, required map[string]struct{}) (bool, error) {
	// A missing snapshot reports nothing running.
	if snapshot == nil {
		return false, nil
	}

	// Collect the running required plugins and fail on a recorded error.
	running := make(map[string]struct{}, len(required))
	for _, plugin := range snapshot.Plugins {
		if plugin == nil || plugin.GetInstanceKey() != "" {
			continue
		}
		pluginID := plugin.GetPluginId()
		if _, ok := required[pluginID]; !ok {
			continue
		}
		if msg := plugin.GetLastErrorMessage(); msg != "" && !plugin.GetRunning() {
			return false, errors.Errorf("plugin %s failed: %s", pluginID, msg)
		}
		if plugin.GetRunning() {
			running[pluginID] = struct{}{}
		}
	}

	// Report false when any required plugin is not running.
	for pluginID := range required {
		if _, ok := running[pluginID]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// setPluginStatus sets a plugin's status state and summary.
func (c *Controller) setPluginStatus(
	pluginID,
	instanceKey string,
	state bldr_plugin.PluginState,
) {
	c.updatePluginStatus(pluginID, instanceKey, state, "", nil, false, false)
}

// setPluginStatusClearingError sets a plugin's status state, clearing any
// prior error.
func (c *Controller) setPluginStatusClearingError(
	pluginID,
	instanceKey string,
	state bldr_plugin.PluginState,
) {
	c.updatePluginStatus(pluginID, instanceKey, state, "", nil, false, true)
}

// recordPluginStatusError records an error against a plugin's status.
func (c *Controller) recordPluginStatusError(
	pluginID,
	instanceKey,
	stage string,
	err error,
) {
	// Ignore nil errors and cancellations.
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	msg := err.Error()
	if stage != "" {
		msg = stage + ": " + msg
	}
	c.updatePluginStatus(
		pluginID,
		instanceKey,
		bldr_plugin.PluginState_PluginState_UNKNOWN,
		msg,
		timestamp.Now(),
		true,
		false,
	)
}

// clearPluginStatusError clears a plugin's recorded error.
func (c *Controller) clearPluginStatusError(pluginID, instanceKey string) {
	c.updatePluginStatus(
		pluginID,
		instanceKey,
		bldr_plugin.PluginState_PluginState_UNKNOWN,
		"",
		nil,
		false,
		true,
	)
}

// clearPluginStatusErrorStage clears a plugin's error once its stage
// advances past the failed stage.
func (c *Controller) clearPluginStatusErrorStage(pluginID, instanceKey, stage string) {
	// Read the plugin's current status entry.
	current := c.pluginStatus.plugin(pluginInstanceKey(pluginID, instanceKey))
	if current == nil {
		return
	}

	// Clear the error only when the recorded stage still matches.
	if stage != "" && !strings.HasPrefix(current.GetLastErrorMessage(), stage+": ") {
		return
	}
	c.clearPluginStatusError(pluginID, instanceKey)
}

// recordPluginManifestRecoveryStatus records manifest selection facts for
// a plugin. executeRev is the build rev of executeManifest, which callers read
// from the fetched Manifest or, before it is fetched, from its ManifestRef meta.
func (c *Controller) recordPluginManifestRecoveryStatus(
	pluginID,
	instanceKey string,
	executeManifest *bldr_manifest.ManifestSnapshot,
	executeRev uint64,
	downloadManifest *bldr_manifest.ManifestSnapshot,
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
) {
	// Build the manifest selection facts outside the status lock.
	row := &bldr_plugin.PluginManifestRecoveryStatus{
		PluginId:                    pluginID,
		InstanceKey:                 instanceKey,
		ExecuteManifestRef:          manifestSnapshotRefString(executeManifest),
		DownloadManifestRef:         manifestSnapshotRefString(downloadManifest),
		SkippedCandidateCount:       countStartupManifestEligibility(candidates, startupManifestEligibilitySkipCandidate),
		SkippedCandidateSummary:     summarizeStartupManifestEligibility(candidates, startupManifestEligibilitySkipCandidate),
		IgnoredCandidateCount:       countStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityIgnored),
		IgnoredCandidateSummary:     summarizeStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityIgnored),
		QuarantinedCandidateCount:   countStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityQuarantined),
		QuarantinedCandidateSummary: summarizeStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityQuarantined),
		ExecuteManifestRev:          executeRev,
	}

	// Store the row for this plugin instance.
	c.pluginStatus.recordRecovery(row)
}

// updatePluginExecutionStatus updates a live instance's execution row.
// apply runs under the status lock and must only edit the new row.
func (c *Controller) updatePluginExecutionStatus(pluginID, instanceKey string, apply func(*bldr_plugin.PluginExecutionStatus)) {
	c.pluginStatus.updatePluginExecutionStatus(pluginID, instanceKey, apply)
}

// updatePluginStatus applies one status mutation to a plugin's entry.
func (c *Controller) updatePluginStatus(
	pluginID, instanceKey string,
	state bldr_plugin.PluginState,
	lastErrorMessage string,
	lastErrorAt *timestamp.Timestamp,
	recordError, clearError bool,
) {
	c.pluginStatus.updatePluginStatus(pluginID, instanceKey, state, lastErrorMessage, lastErrorAt, recordError, clearError)
}

// comparePluginInstance orders plugin instances by plugin ID, then instance key.
func comparePluginInstance(aPluginID, aInstanceKey, bPluginID, bInstanceKey string) int {
	if c := strings.Compare(aPluginID, bPluginID); c != 0 {
		return c
	}
	return strings.Compare(aInstanceKey, bInstanceKey)
}

// manifestSnapshotRefString renders a manifest snapshot's ref as a string.
func manifestSnapshotRefString(snapshot *bldr_manifest.ManifestSnapshot) string {
	if snapshot == nil || snapshot.GetManifestRef() == nil {
		return ""
	}
	return snapshot.GetManifestRef().MarshalB58()
}

// summarizeStartupManifestEligibilityKind builds a summary line for one
// eligibility kind.
func summarizeStartupManifestEligibilityKind(
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
	eligibility bldr_manifest_world.StartupManifestEligibility,
) string {
	return summarizeStartupManifestEligibility(candidates, func(candidate *bldr_manifest_world.StartupManifestCandidateEligibility) bool {
		return candidate != nil && candidate.Eligibility == eligibility
	})
}

// summarizeStartupManifestEligibility builds a summary across all
// tracked eligibility kinds.
func summarizeStartupManifestEligibility(
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
	match func(*bldr_manifest_world.StartupManifestCandidateEligibility) bool,
) string {
	if len(candidates) == 0 {
		return ""
	}
	filtered := make([]*bldr_manifest_world.StartupManifestCandidateEligibility, 0, len(candidates))
	for _, candidate := range candidates {
		if match(candidate) {
			filtered = append(filtered, candidate)
		}
	}
	return bldr_manifest_world.SummarizeStartupManifestEligibility(filtered, maxStartupManifestSkipSummaryItems)
}

// countStartupManifestEligibilityKind counts candidates of one
// eligibility kind.
func countStartupManifestEligibilityKind(
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
	eligibility bldr_manifest_world.StartupManifestEligibility,
) uint32 {
	return countStartupManifestEligibility(candidates, func(candidate *bldr_manifest_world.StartupManifestCandidateEligibility) bool {
		return candidate != nil && candidate.Eligibility == eligibility
	})
}

// countStartupManifestEligibility counts candidates per tracked
// eligibility kind.
func countStartupManifestEligibility(
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
	match func(*bldr_manifest_world.StartupManifestCandidateEligibility) bool,
) uint32 {
	var count uint32
	for _, candidate := range candidates {
		if match(candidate) {
			count++
		}
	}
	return count
}
