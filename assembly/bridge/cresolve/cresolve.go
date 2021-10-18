package bridge_cresolve

import (
	"regexp"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
)

// ControllerID is the controller identifier used to configure this bridge.
const ControllerID = "controllerbus/directive-bridge/cresolve/1"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

// NewControllerResolveBridgeController constructs a directive bridge controller
// which bridges controller-bus implementation lookups.
//
// both fields can be nil
func NewControllerResolveBridgeController(target bus.Bus, configIDRe *regexp.Regexp) *bus_bridge.BusBridge {
	return bus_bridge.NewBusBridge(target, NewControllerResolveFilter(configIDRe))
}

// NewControllerResolveFilter constructs the controller resolve filter func.
// the config id re can be nil to indicate any
func NewControllerResolveFilter(configIDRe *regexp.Regexp) bus_bridge.FilterFn {
	checkConfigID := func(id string) bool {
		if configIDRe == nil {
			return true
		}
		return configIDRe.MatchString(id)
	}

	return func(di directive.Instance) (bool, error) {
		dir := di.GetDirective()
		switch d := dir.(type) {
		case resolver.LoadConfigConstructorByID:
			confID := d.LoadConfigConstructorByIDConfigID()
			if !checkConfigID(confID) {
				return false, nil
			}
		case resolver.LoadFactoryByConfig:
			conf := d.LoadFactoryByConfig()
			if conf == nil || !checkConfigID(conf.GetConfigID()) {
				return false, nil
			}
		default:
			return false, nil
		}

		return true, nil
	}
}
