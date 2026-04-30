//go:build !sql_lite

package bucket

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
)

// NewLookupConfig constructs a new LookupConfig with a config object.
//
// If the config is nil sets Disable: true.
func NewLookupConfig(conf configset.ControllerConfig) (*LookupConfig, error) {
	if conf == nil {
		return &LookupConfig{Disable: true}, nil
	}
	ctrlConf, err := configset_proto.NewControllerConfig(conf, true)
	if err != nil {
		return nil, err
	}
	return &LookupConfig{Controller: ctrlConf}, nil
}
