package bridge_volume

import (
	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/blang/semver"
)

// ControllerID is the controller identifier used to configure this bridge.
const ControllerID = "controllerbus/directive-bridge/volume/1"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

// NewVolumeBridgeController constructs a directive bridge controller which
// bridges hydra volumes.
func NewVolumeBridgeController(target bus.Bus, volumeID string) *bus_bridge.BusBridge {
	return bus_bridge.NewBusBridge(target, NewVolumeFilter(volumeID))
}

// NewVolumeFilter constructs the controller resolve filter func.
// the config id re can be nil to indicate any
func NewVolumeFilter(volumeID string) bus_bridge.FilterFn {
	return func(di directive.Instance) (bool, error) {
		dir := di.GetDirective()
		switch d := dir.(type) {
		case volume.LookupVolume:
			confID := d.LookupVolumeID()
			return confID == volumeID, nil
		case volume.BuildBucketAPI:
			confID := d.BuildBucketAPIVolumeID()
			return confID == volumeID, nil
		case volume.BuildObjectStoreAPI:
			confID := d.BuildObjectStoreAPIVolumeID()
			return confID == volumeID, nil
		case volume.ListBuckets:
			re := d.ListBucketsVolumeIDRe()
			if re != nil {
				if !re.MatchString(volumeID) {
					return false, nil
				}
			}
			return true, nil
		default:
			return false, nil
		}
	}
}
