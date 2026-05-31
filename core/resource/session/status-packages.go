//go:build !js

package resource_session

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/ccontainer"
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	plugin_host_process "github.com/s4wave/spacewave/bldr/plugin/host/process"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

func (r *StatusResource) watchRecoveryPackageChanges(ctx context.Context, notify func()) {
	for _, ctr := range r.findPackageStatusCtrs() {
		ctr := ctr
		go func() {
			current := ctr.GetValue()
			_ = ccontainer.WatchChanges(
				ctx,
				current,
				ctr,
				func(*plugin_host_process.PluginPackageStatusSnapshot) error {
					notify()
					return nil
				},
				nil,
			)
		}()
	}
}

func (r *StatusResource) findPackageStatusCtrs() []ccontainer.Watchable[*plugin_host_process.PluginPackageStatusSnapshot] {
	var out []ccontainer.Watchable[*plugin_host_process.PluginPackageStatusSnapshot]
	for _, ctrl := range r.b.GetControllers() {
		hostCtrl, ok := ctrl.(*plugin_host_controller.Controller)
		if !ok {
			continue
		}
		statusProvider, ok := hostCtrl.GetPluginHost().(interface {
			GetPackageStatusCtr() ccontainer.Watchable[*plugin_host_process.PluginPackageStatusSnapshot]
		})
		if !ok {
			continue
		}
		if ctr := statusProvider.GetPackageStatusCtr(); ctr != nil {
			out = append(out, ctr)
		}
	}
	return out
}

func (r *StatusResource) buildNativePackageRecoveryStatuses() []*s4wave_status.NativePackageRecoveryStatus {
	var out []*s4wave_status.NativePackageRecoveryStatus
	for _, ctrl := range r.b.GetControllers() {
		hostCtrl, ok := ctrl.(*plugin_host_controller.Controller)
		if !ok {
			continue
		}
		statusProvider, ok := hostCtrl.GetPluginHost().(interface {
			PackageStatusSnapshot() []plugin_host_process.PluginPackageStatus
		})
		if !ok {
			continue
		}
		for _, status := range statusProvider.PackageStatusSnapshot() {
			out = append(out, &s4wave_status.NativePackageRecoveryStatus{
				PluginId:     status.PluginID,
				DistDir:      status.DistDir,
				Materialized: status.Materialized,
				Invalidated:  status.Invalidated,
				LastAction:   status.LastAction,
				LastError:    status.LastError,
				UpdatedAt:    formatRecoveryStatusTime(status.UpdatedAt),
			})
		}
	}
	slices.SortFunc(out, func(a, b *s4wave_status.NativePackageRecoveryStatus) int {
		if a.PluginId < b.PluginId {
			return -1
		}
		if a.PluginId > b.PluginId {
			return 1
		}
		return 0
	})
	return out
}
