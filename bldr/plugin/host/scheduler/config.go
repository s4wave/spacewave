package plugin_host_scheduler

import (
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/util/backoff"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(
	instanceKey,
	engineID,
	objectKey,
	volumeID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
	noCopyBucketIDs ...string,
) *Config {
	return &Config{
		InstanceKey: instanceKey,
		EngineId:    engineID,
		ObjectKey:   objectKey,
		PeerId:      peerID,
		VolumeId:    volumeID,

		WatchFetchManifest:   watchFetchManifest,
		DisableStoreManifest: disableStoreManifest,
		DisableCopyManifest:  disableCopyManifest,
		NoCopyBucketIds:      slices.Clone(noCopyBucketIDs),
	}
}

// Validate checks that the configuration is well formed. It does not check
// that the referenced engine, volume, or plugins exist.
func (c *Config) Validate() error {
	// Validate the peer, engine, object key, and volume ids.
	if len(c.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if len(c.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(c.GetVolumeId()) == 0 {
		return volume.ErrVolumeIDEmpty
	}

	// Validate each platform selection policy.
	for _, policy := range c.GetPlatformSelectionPolicies() {
		if policy.GetPlatformId() == "" {
			return errors.New("platform_selection_policies: platform_id is required")
		}
		if slices.Contains(policy.GetAllowedPluginIds(), "") {
			return errors.New("platform_selection_policies: allowed_plugin_ids cannot contain empty values")
		}
		if slices.Contains(policy.GetDeniedPluginIds(), "") {
			return errors.New("platform_selection_policies: denied_plugin_ids cannot contain empty values")
		}
		for _, pluginID := range policy.GetDeniedPluginIds() {
			if slices.Contains(policy.GetAllowedPluginIds(), pluginID) {
				return errors.Errorf(
					"platform_selection_policies: plugin %q cannot be both allowed and denied",
					pluginID,
				)
			}
		}
	}

	// Validate the scheduled plugin ids.
	for _, pluginID := range c.GetPluginIds() {
		if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
			return errors.Wrap(err, "plugin_ids")
		}
	}

	// Validate the materializer plugin id and backoff configs.
	if err := bldr_plugin.ValidatePluginID(c.GetMaterializerPluginId(), true); err != nil {
		return errors.Wrap(err, "materializer_plugin_id")
	}
	if err := c.GetFetchBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "fetch_backoff")
	}
	if err := c.GetExecBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "exec_backoff")
	}

	// Validate the startup wait budget.
	if _, err := c.BuildStartupWaitBudget(); err != nil {
		return err
	}
	return nil
}

// DefaultStartupWaitBudget is the startup wait budget used when the config
// leaves StartupWaitBudgetDur unset or zero.
const DefaultStartupWaitBudget = time.Minute

// BuildStartupWaitBudget gets the StartupWaitBudgetDur and fills the default
// if unset. Rejects negative and unparseable durations.
func (c *Config) BuildStartupWaitBudget() (time.Duration, error) {
	// Parse the configured duration.
	budget, err := confparse.ParseDuration(c.GetStartupWaitBudgetDur())
	if err != nil {
		return 0, errors.Wrap(err, "startup_wait_budget_dur")
	}

	// Reject negative budgets and fill the default.
	if budget < 0 {
		return 0, errors.New("startup_wait_budget_dur cannot be negative")
	}
	if budget == 0 {
		return DefaultStartupWaitBudget, nil
	}
	return budget, nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// BuildExecBackoff gets the ExecBackoff and fills defaults if applicable.
func (c *Config) BuildExecBackoff() *backoff.Backoff {
	return c.buildBackoff(c.GetExecBackoff(), 2100)
}

// SchedulesPlugin reports whether the scheduler loads pluginID.
func (c *Config) SchedulesPlugin(pluginID string) bool {
	if slices.Contains(c.GetExternalPluginIds(), pluginID) {
		return false
	}
	pluginIDs := c.GetPluginIds()
	return len(pluginIDs) == 0 || slices.Contains(pluginIDs, pluginID)
}

// FilterPluginPlatformIDs filters platform IDs through PlatformSelectionPolicies.
func (c *Config) FilterPluginPlatformIDs(pluginID string, platformIDs []string) []string {
	// Without policies or platforms there is nothing to filter.
	policies := c.GetPlatformSelectionPolicies()
	if len(policies) == 0 || len(platformIDs) == 0 {
		return platformIDs
	}

	// Keep the platform ids the plugin is allowed to use.
	filtered := make([]string, 0, len(platformIDs))
	for _, platformID := range platformIDs {
		if c.pluginPlatformAllowed(pluginID, platformID) {
			filtered = append(filtered, platformID)
		}
	}
	return filtered
}

func (c *Config) pluginPlatformAllowed(pluginID, platformID string) bool {
	// The first matching policy decides the result.
	for _, policy := range c.GetPlatformSelectionPolicies() {
		if policy.GetPlatformId() != platformID {
			continue
		}
		if slices.Contains(policy.GetDeniedPluginIds(), pluginID) {
			return false
		}
		allowed := policy.GetAllowedPluginIds()
		return len(allowed) == 0 || slices.Contains(allowed, pluginID)
	}
	return true
}

// BuildFetchBackoff gets the FetchBackoff and fills defaults if applicable.
func (c *Config) BuildFetchBackoff() *backoff.Backoff {
	return c.buildBackoff(c.GetFetchBackoff(), 1200)
}

// buildBackoff clones conf and fills exponential defaults with the given max
// interval when unset.
func (c *Config) buildBackoff(
	conf *backoff.Backoff,
	maxInterval uint32,
) *backoff.Backoff {
	// Clone the configured backoff, allocating when unset.
	backoffConf := conf.CloneVT()
	if backoffConf == nil {
		backoffConf = &backoff.Backoff{}
	}

	// A configured kind is used as-is.
	if backoffConf.BackoffKind != 0 {
		return backoffConf
	}

	// Fill the exponential defaults with the max interval.
	if backoffConf.Exponential == nil {
		backoffConf.Exponential = &backoff.Exponential{}
	}
	backoffConf.BackoffKind = backoff.BackoffKind_BackoffKind_EXPONENTIAL
	backoffConf.Exponential.MaxInterval = maxInterval
	return backoffConf
}

// manifestCopyConcurrencyDefault is the aggregate manifest copy allowance
// used when the config leaves FetchConcurrency unset.
const manifestCopyConcurrencyDefault = 2

// manifestCopyConcurrency returns the configured manifest copy concurrency.
// Zero selects the finite default allowance.
func (c *Config) manifestCopyConcurrency() int {
	if c.GetFetchConcurrency() == 0 {
		return manifestCopyConcurrencyDefault
	}
	return int(c.GetFetchConcurrency())
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
