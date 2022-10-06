package devtool

import (
	"context"
	"os"

	bldr_project "github.com/aperturerobotics/bldr/project"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// StartProjectController reads the config file & starts the project controller.
// Returns the directive reference & controller.
func (a *DevtoolArgs) StartProjectController(
	ctx context.Context,
	b bus.Bus,
	repoRoot string,
	startProject bool,
) (
	controller.Controller,
	directive.Reference,
	error,
) {
	projConfig := &bldr_project.ProjectConfig{}
	configPath := a.ConfigPath
	if configPath != "" {
		projConfYaml, err := os.ReadFile(configPath)
		if err != nil {
			return nil, nil, err
		}
		if err := bldr_project.UnmarshalProjectConfig(projConfYaml, projConfig); err != nil {
			return nil, nil, errors.Wrap(err, "unmarshal project config")
		}
		if err := projConfig.Validate(); err != nil {
			return nil, nil, err
		}
	}

	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(bldr_project_controller.NewConfig(
			repoRoot,
			projConfig,
			startProject,
		)),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return ctrl, ctrlRef, nil
}
