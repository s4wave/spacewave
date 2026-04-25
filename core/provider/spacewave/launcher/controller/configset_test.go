package spacewave_launcher_controller

import (
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// TestLauncherConfigSetValidation verifies that a DistConfig carrying a valid
// launcher_config_set entry passes ConfigSetMap.Validate, and that a
// malformed entry fails. Guards the applyDistConfigSet precondition: Validate
// is called before the set is resolved against the bus.
func TestLauncherConfigSetValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]*configset_proto.ControllerConfig
		wantErr bool
	}{
		{
			name: "valid single controller",
			cfg: map[string]*configset_proto.ControllerConfig{
				"remote-world-block-store": {
					Id:  "hydra/block/store/kvfile/http",
					Rev: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "empty id rejected",
			cfg: map[string]*configset_proto.ControllerConfig{
				"no-id": {Rev: 1},
			},
			wantErr: true,
		},
		{
			name:    "empty map valid",
			cfg:     map[string]*configset_proto.ControllerConfig{},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			distConf := &spacewave_launcher.DistConfig{
				ProjectId:         "spacewave",
				Rev:               1,
				LauncherConfigSet: tc.cfg,
			}
			cs := configset_proto.ConfigSetMap(distConf.GetLauncherConfigSet())
			err := cs.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
