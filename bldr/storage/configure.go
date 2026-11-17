package storage

import (
	"github.com/aperturerobotics/controllerbus/config"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
)

// ConfigureVolumeFunc adjusts in place the config a Storage built for the
// Volume id, before the Volume opens.
type ConfigureVolumeFunc = func(id string, conf config.Config)

// WithConfigureVolume returns st with each Volume config it builds passed
// through configure.
func WithConfigureVolume(st Storage, configure ConfigureVolumeFunc) Storage {
	return &configuredStorage{Storage: st, configure: configure}
}

// configuredStorage is a Storage whose Volume configs pass through configure.
type configuredStorage struct {
	Storage
	// configure adjusts each config the Storage builds.
	configure ConfigureVolumeFunc
}

// BuildVolumeConfig builds the config for the Volume id and configures it.
func (s *configuredStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	conf, err := s.Storage.BuildVolumeConfig(id, baseVolCtrlConf)
	if err != nil || conf == nil {
		return conf, err
	}
	s.configure(id, conf)
	return conf, nil
}
