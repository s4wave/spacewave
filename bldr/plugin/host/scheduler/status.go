package plugin_host_scheduler

import (
	"context"
	"slices"
	"strings"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// PluginStatusSnapshot describes the scheduler's current plugin instances.
type PluginStatusSnapshot struct {
	Plugins []*bldr_plugin.PluginStatus
}

// GetPluginStatusCtr returns the scheduler's live plugin-status snapshot.
func (c *Controller) GetPluginStatusCtr() ccontainer.Watchable[*PluginStatusSnapshot] {
	return c.pluginStatusCtr
}

func (c *Controller) setPluginStatus(
	pluginID,
	instanceKey string,
	state bldr_plugin.PluginState,
) {
	c.updatePluginStatus(pluginID, instanceKey, state, "", nil, false, false)
}

func (c *Controller) setPluginStatusClearingError(
	pluginID,
	instanceKey string,
	state bldr_plugin.PluginState,
) {
	c.updatePluginStatus(pluginID, instanceKey, state, "", nil, false, true)
}

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
	return &PluginStatusSnapshot{Plugins: plugins}
}

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
	return true
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
