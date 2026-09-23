package resource_root_controller

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_configtype_registry "github.com/s4wave/spacewave/core/resource/configtype/registry"
	resource_objecttype_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	resource_quickstart_registry "github.com/s4wave/spacewave/core/resource/quickstart/registry"
	resource_viewer_registry "github.com/s4wave/spacewave/core/resource/viewer/registry"
	resource_worldop_registry "github.com/s4wave/spacewave/core/resource/worldop/registry"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// registries are the capability registries plugins fill through the root
// resource. Plugins register with the root of their host installation, so a
// nested app serves its parent's registries to reach the same plugins.
type registries struct {
	// viewer holds object viewers.
	viewer *resource_viewer_registry.ViewerRegistryResource
	// objectType holds World ObjectTypes.
	objectType *resource_objecttype_registry.ObjectTypeRegistryResource
	// worldOp holds World operation types.
	worldOp *resource_worldop_registry.WorldOpRegistryResource
	// configType holds config types.
	configType *resource_configtype_registry.ConfigTypeRegistryResource
	// quickstart holds Quickstarts and runs their seed handlers with the bus
	// of the installation that created it.
	quickstart *resource_quickstart_registry.QuickstartRegistryResource
	// objectWizard holds object creation wizards.
	objectWizard *s4wave_wizard.WizardRegistryResource
}

// newRegistries constructs empty registries. b is where Quickstart seed
// handlers load their plugins.
func newRegistries(le *logrus.Entry, b bus.Bus) *registries {
	return &registries{
		viewer:       resource_viewer_registry.NewViewerRegistryResource(),
		objectType:   resource_objecttype_registry.NewObjectTypeRegistryResource(),
		worldOp:      resource_worldop_registry.NewWorldOpRegistryResource(),
		configType:   resource_configtype_registry.NewConfigTypeRegistryResource(),
		quickstart:   resource_quickstart_registry.NewQuickstartRegistryResource(le, b),
		objectWizard: s4wave_wizard.NewWizardRegistryResource(),
	}
}

// register serves every registry on the root resource mux.
func (r *registries) register(mux srpc.Mux) error {
	if err := s4wave_viewer_registry.SRPCRegisterViewerRegistryResourceService(mux, r.viewer); err != nil {
		return err
	}
	if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeRegistryResourceService(mux, r.objectType); err != nil {
		return err
	}
	if err := s4wave_worldop_registry.SRPCRegisterWorldOpRegistryResourceService(mux, r.worldOp); err != nil {
		return err
	}
	if err := s4wave_configtype_registry.SRPCRegisterConfigTypeRegistryResourceService(mux, r.configType); err != nil {
		return err
	}
	if err := s4wave_quickstart_registry.SRPCRegisterQuickstartRegistryResourceService(mux, r.quickstart); err != nil {
		return err
	}
	return s4wave_wizard.SRPCRegisterObjectWizardRegistryResourceService(mux, r.objectWizard)
}
