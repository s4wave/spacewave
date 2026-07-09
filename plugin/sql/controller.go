package sql_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/blocktype"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// ControllerID is the controller identifier.
const ControllerID = "plugin/sql"

// PluginID is the manifest id for the SQL plugin.
const PluginID = "spacewave-sql"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

// Controller registers SQL domain handlers and serves them as a plugin resource.
type Controller struct {
	*bus.BusController[*Config]

	mux srpc.Invoker
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		"spacewave sql domain plugin controller",
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			handler := &SQLHandler{le: base.GetLogger(), b: b}
			rootMux := resource_server.NewResourceMux(
				func(mux srpc.Mux) error {
					return s4wave_objecttype_registry.SRPCRegisterObjectTypeHandlerService(mux, handler)
				},
				func(mux srpc.Mux) error {
					return s4wave_worldop_registry.SRPCRegisterWorldOpHandlerService(mux, handler)
				},
				func(mux srpc.Mux) error {
					return s4wave_quickstart_registry.SRPCRegisterQuickstartHandlerService(mux, handler)
				},
			)
			serverMux := srpc.NewMux()
			if err := resource_server.NewResourceServer(rootMux).Register(serverMux); err != nil {
				return nil, err
			}
			return &Controller{BusController: base, mux: serverMux}, nil
		},
	)
}

// Execute registers SQL ObjectTypes, WorldOps, viewers, and Quickstart metadata.
func (c *Controller) Execute(ctx context.Context) error {
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, c.GetBus(), "spacewave-core")
	if err != nil {
		return err
	}
	defer resources.Release()

	rootRef := resources.Client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		return err
	}
	defer rootRef.Release()

	refs, err := c.registerSQL(ctx, resources.Client, rootClient)
	if err != nil {
		releaseRefs(refs)
		return err
	}
	defer releaseRefs(refs)

	<-ctx.Done()
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == resource.SRPCResourceServiceServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	case blocktype.LookupBlockType:
		typeID := d.LookupBlockTypeID()
		if typeID == "" {
			return nil, nil
		}
		return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
			blockType, err := lookupSQLBlockType(typeID)
			if err != nil {
				return err
			}
			if blockType != nil {
				_, _ = handler.AddValue(blockType)
			}
			return nil
		}), nil)
	}
	return nil, nil
}

func (c *Controller) registerSQL(
	ctx context.Context,
	client *resource_client.Client,
	rootClient srpc.Client,
) ([]resource_client.ResourceRef, error) {
	var refs []resource_client.ResourceRef
	retain := func(label string, resourceID uint32) error {
		if resourceID == 0 {
			return errors.Errorf("sql plugin: %s registration returned zero resource id", label)
		}
		refs = append(refs, client.CreateResourceReference(resourceID))
		return nil
	}

	otSvc := s4wave_objecttype_registry.NewSRPCObjectTypeRegistryResourceServiceClient(rootClient)
	for _, typeID := range sqlObjectTypeIDs {
		resp, err := otSvc.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
			TypeId:   typeID,
			PluginId: PluginID,
		})
		if err != nil {
			return refs, err
		}
		if err := retain("object type "+typeID, resp.GetResourceId()); err != nil {
			return refs, err
		}
	}

	woSvc := s4wave_worldop_registry.NewSRPCWorldOpRegistryResourceServiceClient(rootClient)
	for _, opID := range sqlWorldOpIDs {
		resp, err := woSvc.RegisterWorldOp(ctx, &s4wave_worldop_registry.RegisterWorldOpRequest{
			OperationTypeId: opID,
			PluginId:        PluginID,
		})
		if err != nil {
			return refs, err
		}
		if err := retain("world op "+opID, resp.GetResourceId()); err != nil {
			return refs, err
		}
	}

	qsSvc := s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceClient(rootClient)
	quickstart, err := qsSvc.RegisterQuickstart(ctx, &s4wave_quickstart_registry.RegisterQuickstartRequest{
		Registration: &s4wave_quickstart_registry.QuickstartRegistration{
			QuickstartId:      SQLQuickstartID,
			PluginId:          PluginID,
			Name:              "SQL Database",
			Description:       "Seed a SQL database with sample tables and a query.",
			Category:          "Data",
			IconName:          "database",
			SpaceName:         "My SQL Database",
			RequiredPluginIds: []string{PluginID},
		},
	})
	if err != nil {
		return refs, err
	}
	if err := retain("quickstart "+SQLQuickstartID, quickstart.GetResourceId()); err != nil {
		return refs, err
	}

	viewerSvc := s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceClient(rootClient)
	for _, viewer := range sqlViewerRegistrations {
		resp, err := viewerSvc.RegisterViewer(ctx, &s4wave_viewer_registry.RegisterViewerRequest{Registration: viewer})
		if err != nil {
			return refs, err
		}
		if err := retain("viewer "+viewer.GetTypeId(), resp.GetResourceId()); err != nil {
			return refs, err
		}
	}

	return refs, nil
}

func releaseRefs(refs []resource_client.ResourceRef) {
	for i := len(refs) - 1; i >= 0; i-- {
		refs[i].Release()
	}
}

// _ is a type assertion.
var _ controller.Controller = ((*Controller)(nil))
