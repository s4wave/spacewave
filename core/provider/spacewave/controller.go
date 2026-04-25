package provider_spacewave

import (
	"github.com/blang/semver/v4"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_controller "github.com/s4wave/spacewave/core/provider/controller"
)

// ControllerID is the controller id.
const ControllerID = "provider/spacewave"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "spacewave cloud provider"

// Controller is the provider controller type.
type Controller = provider_controller.ProviderController

// _ is a type assertion
var _ provider.ProviderController = (*Controller)(nil)
