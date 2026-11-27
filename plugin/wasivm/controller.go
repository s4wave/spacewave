// Package wasivm_plugin serves WasiVM virtual machines to the Spaces that
// load the spacewave-wasivm plugin. It registers the vm/wasivm ObjectType, its
// operations, viewer and creation wizard, and runs each VM's kernel in the
// plugin process.
package wasivm_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	"github.com/s4wave/spacewave/db/blocktype"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_plugin_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// ControllerID is the controller identifier.
const ControllerID = "plugin/wasivm"

// PluginID is the manifest ID of the WasiVM plugin.
const PluginID = "spacewave-wasivm"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

// wizardTypeID is the ObjectType of the wizard that creates a WasiVM.
const wizardTypeID = "wizard/vm/wasivm"

// ViewerEntrypoint is the bundled viewer of a WasiVM.
//
//bldr:esbuild WasiVmViewer.tsx
var ViewerEntrypoint bldr_web_bundler.WebBundlerOutput

// WizardEntrypoint is the bundled wizard that creates a WasiVM.
//
//bldr:esbuild WasiVmWizard.tsx
var WizardEntrypoint bldr_web_bundler.WebBundlerOutput

// Controller registers WasiVM with the core plugin and serves its resources.
type Controller struct {
	*bus.BusController[*Config]
	// Holder publishes the registrations and reports when they are ready.
	*s4wave_plugin_registration.Holder

	// mux serves the plugin's resource service.
	mux srpc.Invoker
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		"WasiVM virtual machine plugin",
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			// Serve the ObjectType, its bodies and the operations of WasiVM.
			h := newHandler(base.GetLogger(), base.GetBus())
			rootMux := resource_server.NewResourceMux(
				func(mux srpc.Mux) error {
					return s4wave_objecttype_registry.SRPCRegisterObjectTypeHandlerService(mux, h)
				},
				func(mux srpc.Mux) error {
					return s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(mux, s4wave_worldop_registry.NewHandler(base.GetLogger(), wasiVMSchema.LookupOp))
				},
				func(mux srpc.Mux) error {
					return s4wave_objecttype_registry.SRPCRegisterObjectTypeBodyService(mux, s4wave_objecttype_registry.NewBodyHandler(wasiVMSchema.LookupBlockType))
				},
			)

			// Expose them through the resource server.
			serverMux := srpc.NewMux()
			resourceSrv := resource_server.NewResourceServer(rootMux, ControllerID)
			if err := resourceSrv.Register(serverMux); err != nil {
				return nil, err
			}
			return &Controller{
				BusController: base,
				Holder:        s4wave_plugin_registration.NewHolder(register),
				mux:           serverMux,
			}, nil
		},
	)
}

// Execute registers WasiVM with the core plugin until ctx ends.
func (c *Controller) Execute(ctx context.Context) error {
	return c.Hold(ctx, c.GetBus())
}

// HandleDirective serves the plugin's resource service and the WasiVM block
// types.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == resource.SRPCResourceServiceServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	case blocktype.LookupBlockType:
		bt, err := wasiVMSchema.LookupBlockType(ctx, d.LookupBlockTypeID())
		if err != nil || bt == nil {
			return nil, err
		}
		return directive.R(directive.NewValueResolver([]blocktype.BlockType{bt}), nil)
	}
	return nil, nil
}

// register registers the WasiVM ObjectType, operations, viewers and wizard
// through scope. It returns the registrations made, even on error.
func register(
	ctx context.Context,
	client *resource_client.Client,
	scope srpc.Client,
) ([]resource_client.ResourceRef, error) {
	// Retain each registration until the caller releases it.
	var refs []resource_client.ResourceRef
	retain := func(what string, resourceID uint32, err error) error {
		if err != nil {
			return errors.Wrapf(err, "register %s", what)
		}
		if resourceID == 0 {
			return errors.Errorf("register %s: no registration returned", what)
		}
		refs = append(refs, client.CreateResourceReference(resourceID))
		return nil
	}

	// Register the ObjectType and its operations.
	ot, err := s4wave_objecttype_registry.NewSRPCObjectTypeRegistryResourceServiceClient(scope).
		RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
			TypeId:   s4wave_vm.WasiVmTypeID,
			PluginId: PluginID,
			Metadata: &s4wave_objecttype_registry.ObjectTypeMetadata{
				DisplayName: "WebAssembly VM",
				IconName:    "LuTerminal",
			},
		})
	if err := retain("object type", ot.GetResourceId(), err); err != nil {
		return refs, err
	}
	woSvc := s4wave_worldop_registry.NewSRPCWorldOpRegistryResourceServiceClient(scope)
	for _, opID := range []string{
		s4wave_vm.CreateWasiVmOpTypeID,
		s4wave_vm.SetWasiVmStateOpTypeID,
		s4wave_vm.CreateWasiVmImageOpTypeID,
		s4wave_vm.CreateWasiVmPackageOpTypeID,
	} {
		resp, err := woSvc.RegisterWorldOp(ctx, &s4wave_worldop_registry.RegisterWorldOpRequest{
			OperationTypeId: opID,
			PluginId:        PluginID,
		})
		if err := retain("operation "+opID, resp.GetResourceId(), err); err != nil {
			return refs, err
		}
	}

	// Register the viewer and the wizard viewer at the scripts of this build.
	info := bldr_plugin.GetPluginContextInfo(ctx)
	viewerSvc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(scope)
	for _, viewer := range []*s4wave_viewer_registry.ViewerRegistration{{
		TypeId:      s4wave_vm.WasiVmTypeID,
		ViewerName:  "WebAssembly VM",
		ScriptPath:  info.PinAssetsHref(ViewerEntrypoint.GetEntrypointHref()),
		ComponentId: "spacewave.vm.wasivm",
		Surface:     s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	}, {
		TypeId:      wizardTypeID,
		ViewerName:  "Create WebAssembly VM",
		ScriptPath:  info.PinAssetsHref(WizardEntrypoint.GetEntrypointHref()),
		ComponentId: "spacewave.wizard.wasivm",
		Surface:     s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_WEB,
	}} {
		resp, err := viewerSvc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{Registration: viewer})
		if err := retain("viewer "+viewer.GetTypeId(), resp.GetResourceId(), err); err != nil {
			return refs, err
		}
	}

	// Register the wizard that creates a WasiVM.
	wizard, err := s4wave_wizard.NewSRPCObjectWizardRegistryResourceServiceClient(scope).
		RegisterWizard(ctx, &s4wave_wizard.RegisterWizardRequest{Wizard: &s4wave_wizard.ObjectWizard{
			TypeId:             s4wave_vm.WasiVmTypeID,
			PluginId:           PluginID,
			DisplayName:        "WebAssembly VM",
			Description:        "Run Linux programs built for WebAssembly on a Space filesystem",
			Category:           "VM",
			IconName:           "LuTerminal",
			CreateOpId:         s4wave_vm.CreateWasiVmOpTypeID,
			DefaultNamePattern: "VM",
			KeyPrefix:          "vm/wasivm/",
			Persistent:         true,
			WizardTypeId:       wizardTypeID,
			Experimental:       true,
		}})
	if err := retain("wizard", wizard.GetResourceId(), err); err != nil {
		return refs, err
	}
	return refs, nil
}

// _ is a type assertion.
var (
	_ controller.Controller                  = (*Controller)(nil)
	_ bldr_plugin.InitialCapabilityRegistrar = (*Controller)(nil)
)
