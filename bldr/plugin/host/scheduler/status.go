package plugin_host_scheduler

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// PluginStatusSnapshot describes the scheduler's current plugin instances.
// PluginStatusSnapshot is the scheduler's live plugin-status snapshot.
type PluginStatusSnapshot struct {
	// Plugins lists the per-plugin statuses.
	Plugins []*bldr_plugin.PluginStatus
	// ManifestRecovery lists per-plugin manifest selection facts.
	ManifestRecovery []*PluginManifestRecoveryStatus
}

// PluginManifestRecoveryStatus describes the scheduler-owned retained Manifest
// selection and eligibility facts for one plugin instance.
type PluginManifestRecoveryStatus struct {
	// PluginID is the plugin's id.
	PluginID string
	// InstanceKey is the plugin's instance key.
	InstanceKey string
	// ExecuteManifestRef is the ref of the manifest selected for execution.
	ExecuteManifestRef string
	// DownloadManifestRef is the ref of the manifest selected for download.
	DownloadManifestRef string
	// SkippedCandidateCount is how many candidates were skipped.
	SkippedCandidateCount int
	// SkippedCandidateSummary summarizes the skipped candidates.
	SkippedCandidateSummary string
	// IgnoredCandidateCount is how many candidates were ignored.
	IgnoredCandidateCount int
	// IgnoredCandidateSummary summarizes the ignored candidates.
	IgnoredCandidateSummary string
	// QuarantinedCandidateCount is how many candidates were quarantined.
	QuarantinedCandidateCount int
	// QuarantinedCandidateSummary summarizes the quarantined candidates.
	QuarantinedCandidateSummary string
}

// GetPluginStatusCtr returns the scheduler's live plugin-status snapshot.
func (c *Controller) GetPluginStatusCtr() ccontainer.Watchable[*PluginStatusSnapshot] {
	return c.pluginStatusCtr
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
	snapshot := c.pluginStatusCtr.GetValue()
	if snapshot == nil {
		return false
	}
	for _, plugin := range snapshot.Plugins {
		if plugin.GetPluginId() == pluginID && plugin.GetRunning() {
			return true
		}
	}
	return false
}

// WaitPluginsRunning waits until all shared instances in pluginIDs report
// running, or returns the first scheduler error recorded for one of them.
func (c *Controller) WaitPluginsRunning(ctx context.Context, pluginIDs []string) error {
	if len(pluginIDs) == 0 {
		return nil
	}
	if c.pluginStatusCtr == nil {
		return errors.New("plugin status controller is not initialized")
	}

	required := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		if pluginID != "" {
			required[pluginID] = struct{}{}
		}
	}
	if len(required) == 0 {
		return nil
	}

	var current *PluginStatusSnapshot
	for {
		next, err := c.pluginStatusCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next

		running, err := pluginsRunningOrError(current, required)
		if err != nil || running {
			return err
		}
	}
}

// pluginsRunningOrError reports whether every required plugin is running,
// returning an error naming any missing one.
func pluginsRunningOrError(snapshot *PluginStatusSnapshot, required map[string]struct{}) (bool, error) {
	if snapshot == nil {
		return false, nil
	}

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
	key := pluginInstanceKey(pluginID, instanceKey)
	c.pluginStatusMtx.Lock()
	current := c.pluginStatus[key]
	c.pluginStatusMtx.Unlock()
	if current == nil {
		return
	}
	if stage != "" && !strings.HasPrefix(current.GetLastErrorMessage(), stage+": ") {
		return
	}
	c.clearPluginStatusError(pluginID, instanceKey)
}

// recordPluginManifestRecoveryStatus records manifest selection facts for
// a plugin.
func (c *Controller) recordPluginManifestRecoveryStatus(
	pluginID,
	instanceKey string,
	executeManifest,
	downloadManifest *bldr_manifest.ManifestSnapshot,
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
) {
	key := pluginInstanceKey(pluginID, instanceKey)
	c.pluginStatusMtx.Lock()
	if c.pluginManifestRecoveryStatus == nil {
		c.pluginManifestRecoveryStatus = make(map[string]*PluginManifestRecoveryStatus)
	}
	c.pluginManifestRecoveryStatus[key] = &PluginManifestRecoveryStatus{
		PluginID:                    pluginID,
		InstanceKey:                 instanceKey,
		ExecuteManifestRef:          manifestSnapshotRefString(executeManifest),
		DownloadManifestRef:         manifestSnapshotRefString(downloadManifest),
		SkippedCandidateCount:       countStartupManifestEligibility(candidates, startupManifestEligibilitySkipCandidate),
		SkippedCandidateSummary:     summarizeStartupManifestEligibility(candidates, startupManifestEligibilitySkipCandidate),
		IgnoredCandidateCount:       countStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityIgnored),
		IgnoredCandidateSummary:     summarizeStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityIgnored),
		QuarantinedCandidateCount:   countStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityQuarantined),
		QuarantinedCandidateSummary: summarizeStartupManifestEligibilityKind(candidates, bldr_manifest_world.StartupManifestEligibilityQuarantined),
	}
	snapshot := c.buildPluginStatusSnapshotLocked()
	c.pluginStatusMtx.Unlock()
	if c.pluginStatusCtr != nil {
		c.pluginStatusCtr.SetValue(snapshot)
	}
}

// updatePluginStatus applies one status mutation to a plugin's entry.
func (c *Controller) updatePluginStatus(
	pluginID,
	instanceKey string,
	state bldr_plugin.PluginState,
	lastErrorMessage string,
	lastErrorAt *timestamp.Timestamp,
	recordError,
	clearError bool,
) {
	key := pluginInstanceKey(pluginID, instanceKey)
	c.pluginStatusMtx.Lock()
	if c.pluginStatus == nil {
		c.pluginStatus = make(map[string]*bldr_plugin.PluginStatus)
	}
	current := c.pluginStatus[key]
	if state == bldr_plugin.PluginState_PluginState_UNKNOWN && !recordError && !clearError {
		delete(c.pluginStatus, key)
		delete(c.pluginManifestRecoveryStatus, key)
	} else if state == bldr_plugin.PluginState_PluginState_UNKNOWN && clearError && current == nil {
		// Successful completion can race with plugin reference cleanup. If the
		// instance is already gone, clearing metadata should not recreate it.
	} else {
		if state == bldr_plugin.PluginState_PluginState_UNKNOWN {
			state = bldr_plugin.PluginState_PluginState_REQUESTED
			if current != nil {
				state = current.State
			}
		}
		if !recordError {
			if clearError {
				lastErrorMessage = ""
				lastErrorAt = nil
			} else if current != nil {
				lastErrorMessage = current.LastErrorMessage
				lastErrorAt = current.LastErrorAt
			}
		}
		c.pluginStatus[key] = &bldr_plugin.PluginStatus{
			PluginId:         pluginID,
			InstanceKey:      instanceKey,
			Running:          state == bldr_plugin.PluginState_PluginState_RUNNING,
			State:            state,
			LastErrorMessage: lastErrorMessage,
			LastErrorAt:      cloneTimestamp(lastErrorAt),
		}
	}
	snapshot := c.buildPluginStatusSnapshotLocked()
	c.pluginStatusMtx.Unlock()
	if c.pluginStatusCtr != nil {
		c.pluginStatusCtr.SetValue(snapshot)
	}
}

// buildPluginStatusSnapshotLocked builds the current snapshot. Caller must
// hold pluginStatusMtx.
func (c *Controller) buildPluginStatusSnapshotLocked() *PluginStatusSnapshot {
	plugins := make([]*bldr_plugin.PluginStatus, 0, len(c.pluginStatus))
	for _, plugin := range c.pluginStatus {
		if plugin == nil {
			continue
		}
		plugins = append(plugins, &bldr_plugin.PluginStatus{
			PluginId:         plugin.PluginId,
			InstanceKey:      plugin.InstanceKey,
			Running:          plugin.Running,
			State:            plugin.State,
			LastErrorMessage: plugin.LastErrorMessage,
			LastErrorAt:      cloneTimestamp(plugin.LastErrorAt),
		})
	}
	slices.SortFunc(plugins, func(a, b *bldr_plugin.PluginStatus) int {
		if a.PluginId < b.PluginId {
			return -1
		}
		if a.PluginId > b.PluginId {
			return 1
		}
		if a.InstanceKey < b.InstanceKey {
			return -1
		}
		if a.InstanceKey > b.InstanceKey {
			return 1
		}
		return 0
	})
	recovery := make([]*PluginManifestRecoveryStatus, 0, len(c.pluginManifestRecoveryStatus))
	for _, row := range c.pluginManifestRecoveryStatus {
		if row == nil {
			continue
		}
		recovery = append(recovery, clonePluginManifestRecoveryStatus(row))
	}
	slices.SortFunc(recovery, func(a, b *PluginManifestRecoveryStatus) int {
		if a.PluginID < b.PluginID {
			return -1
		}
		if a.PluginID > b.PluginID {
			return 1
		}
		if a.InstanceKey < b.InstanceKey {
			return -1
		}
		if a.InstanceKey > b.InstanceKey {
			return 1
		}
		return 0
	})
	return &PluginStatusSnapshot{Plugins: plugins, ManifestRecovery: recovery}
}

// pluginStatusSnapshotEqual reports whether two snapshots are equal.
func pluginStatusSnapshotEqual(a, b *PluginStatusSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Plugins) != len(b.Plugins) {
		return false
	}
	for i, ap := range a.Plugins {
		bp := b.Plugins[i]
		if ap.PluginId != bp.PluginId ||
			ap.InstanceKey != bp.InstanceKey ||
			ap.Running != bp.Running ||
			ap.State != bp.State ||
			ap.LastErrorMessage != bp.LastErrorMessage ||
			!timestampEqual(ap.LastErrorAt, bp.LastErrorAt) {
			return false
		}
	}
	return slices.EqualFunc(a.ManifestRecovery, b.ManifestRecovery, pluginManifestRecoveryStatusEqual)
}

// clonePluginManifestRecoveryStatus deep-clones a recovery status row.
func clonePluginManifestRecoveryStatus(row *PluginManifestRecoveryStatus) *PluginManifestRecoveryStatus {
	if row == nil {
		return nil
	}
	next := *row
	return &next
}

// pluginManifestRecoveryStatusEqual reports whether two recovery rows are
// equal.
func pluginManifestRecoveryStatusEqual(a, b *PluginManifestRecoveryStatus) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.PluginID == b.PluginID &&
		a.InstanceKey == b.InstanceKey &&
		a.ExecuteManifestRef == b.ExecuteManifestRef &&
		a.DownloadManifestRef == b.DownloadManifestRef &&
		a.SkippedCandidateCount == b.SkippedCandidateCount &&
		a.SkippedCandidateSummary == b.SkippedCandidateSummary &&
		a.IgnoredCandidateCount == b.IgnoredCandidateCount &&
		a.IgnoredCandidateSummary == b.IgnoredCandidateSummary &&
		a.QuarantinedCandidateCount == b.QuarantinedCandidateCount &&
		a.QuarantinedCandidateSummary == b.QuarantinedCandidateSummary
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
) int {
	return countStartupManifestEligibility(candidates, func(candidate *bldr_manifest_world.StartupManifestCandidateEligibility) bool {
		return candidate != nil && candidate.Eligibility == eligibility
	})
}

// countStartupManifestEligibility counts candidates per tracked
// eligibility kind.
func countStartupManifestEligibility(
	candidates []*bldr_manifest_world.StartupManifestCandidateEligibility,
	match func(*bldr_manifest_world.StartupManifestCandidateEligibility) bool,
) int {
	var count int
	for _, candidate := range candidates {
		if match(candidate) {
			count++
		}
	}
	return count
}

func cloneTimestamp(ts *timestamp.Timestamp) *timestamp.Timestamp {
	if ts == nil {
		return nil
	}
	return ts.CloneVT()
}

func timestampEqual(a, b *timestamp.Timestamp) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.EqualVT(b)
}
