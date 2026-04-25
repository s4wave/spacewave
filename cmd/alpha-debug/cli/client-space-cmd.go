//go:build !js

package cli

import (
	"context"
	"os"
	"strconv"

	appcli "github.com/aperturerobotics/cli"
	"github.com/pkg/errors"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// SpaceArgs contains space subcommand arguments.
type SpaceArgs struct {
	client *ClientArgs

	// SpaceID is the target space ID.
	SpaceID string
	// PluginID is the target plugin manifest ID.
	PluginID string
	// Approved is the approval state for set-plugin-approval.
	Approved bool
}

// BuildSpaceCommand returns the space command with subcommands.
func (a *ClientArgs) BuildSpaceCommand() *appcli.Command {
	sa := &SpaceArgs{client: a}
	return &appcli.Command{
		Name:  "space",
		Usage: "manage a Space (plugins, settings)",
		Flags: append(a.BuildFlags(), &appcli.StringFlag{
			Name:        "space-id",
			Usage:       "ID of the Space to manage",
			Required:    true,
			Destination: &sa.SpaceID,
		}),
		Subcommands: []*appcli.Command{
			{
				Name:  "settings",
				Usage: "manage space settings",
				Subcommands: []*appcli.Command{
					{
						Name:  "add-plugin",
						Usage: "add a plugin manifest ID to the space settings",
						Flags: []appcli.Flag{
							&appcli.StringFlag{
								Name:        "plugin-id",
								Usage:       "manifest ID of the plugin to add",
								Required:    true,
								Destination: &sa.PluginID,
							},
						},
						Action: sa.RunAddPlugin,
					},
					{
						Name:  "remove-plugin",
						Usage: "remove a plugin manifest ID from the space settings",
						Flags: []appcli.Flag{
							&appcli.StringFlag{
								Name:        "plugin-id",
								Usage:       "manifest ID of the plugin to remove",
								Required:    true,
								Destination: &sa.PluginID,
							},
						},
						Action: sa.RunRemovePlugin,
					},
				},
			},
			{
				Name:  "set-plugin-approval",
				Usage: "set the approval state for a plugin in this space",
				Flags: []appcli.Flag{
					&appcli.StringFlag{
						Name:        "plugin-id",
						Usage:       "manifest ID of the plugin",
						Required:    true,
						Destination: &sa.PluginID,
					},
					&appcli.BoolFlag{
						Name:        "approved",
						Usage:       "approval state (true to approve, false to deny)",
						Value:       true,
						Destination: &sa.Approved,
					},
				},
				Action: sa.RunSetPluginApproval,
			},
			{
				Name:   "status",
				Usage:  "show space state including settings and world contents",
				Action: sa.RunStatus,
			},
			{
				Name:   "plugins",
				Usage:  "show plugin statuses (mirrors what the UI sees)",
				Action: sa.RunPlugins,
			},
		},
	}
}

// RunAddPlugin adds a plugin to the space settings.
func (sa *SpaceArgs) RunAddPlugin(c *appcli.Context) error {
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	_, err = spaceSvc.AddSpacePlugin(ctx, &s4wave_space.AddSpacePluginRequest{
		PluginId: sa.PluginID,
	})
	if err != nil {
		return errors.Wrap(err, "add space plugin")
	}
	os.Stdout.WriteString("added plugin " + sa.PluginID + " to space settings\n")
	return nil
}

// RunRemovePlugin removes a plugin from the space settings.
func (sa *SpaceArgs) RunRemovePlugin(c *appcli.Context) error {
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	_, err = spaceSvc.RemoveSpacePlugin(ctx, &s4wave_space.RemoveSpacePluginRequest{
		PluginId: sa.PluginID,
	})
	if err != nil {
		return errors.Wrap(err, "remove space plugin")
	}
	os.Stdout.WriteString("removed plugin " + sa.PluginID + " from space settings\n")
	return nil
}

// RunSetPluginApproval sets the approval state for a plugin.
func (sa *SpaceArgs) RunSetPluginApproval(c *appcli.Context) error {
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// Mount space contents to get the SpaceContentsResource.
	contentsResp, err := spaceSvc.MountSpaceContents(ctx, &s4wave_space.MountSpaceContentsRequest{})
	if err != nil {
		return errors.Wrap(err, "mount space contents")
	}

	// Get a client for the contents resource.
	contentsClient, err := sa.getResourceClient(ctx, contentsResp.GetResourceId())
	if err != nil {
		return errors.Wrap(err, "space contents client")
	}
	contentsSvc := s4wave_space.NewSRPCSpaceContentsResourceServiceClient(contentsClient)

	_, err = contentsSvc.SetPluginApproval(ctx, &s4wave_space.SetPluginApprovalRequest{
		PluginId: sa.PluginID,
		Approved: sa.Approved,
	})
	if err != nil {
		return errors.Wrap(err, "set plugin approval")
	}
	state := "approved"
	if !sa.Approved {
		state = "denied"
	}
	os.Stdout.WriteString("plugin " + sa.PluginID + " " + state + "\n")
	return nil
}

// RunStatus prints the SpaceState including settings and world contents.
func (sa *SpaceArgs) RunStatus(c *appcli.Context) error {
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	strm, err := spaceSvc.WatchSpaceState(ctx, &s4wave_space.WatchSpaceStateRequest{})
	if err != nil {
		return errors.Wrap(err, "watch space state")
	}
	defer strm.Close()

	state, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv space state")
	}

	w := os.Stdout
	w.WriteString("ready: " + strconv.FormatBool(state.GetReady()) + "\n")

	settings := state.GetSettings()
	if settings == nil {
		w.WriteString("settings: <nil>\n")
	}
	if settings != nil {
		pids := settings.GetPluginIds()
		w.WriteString("settings.plugin_ids (" + strconv.Itoa(len(pids)) + "):\n")
		for _, pid := range pids {
			w.WriteString("  - " + pid + "\n")
		}
	}

	wc := state.GetWorldContents()
	if wc != nil {
		objs := wc.GetObjects()
		w.WriteString("world objects (" + strconv.Itoa(len(objs)) + "):\n")
		for _, obj := range objs {
			w.WriteString("  - key=" + obj.GetObjectKey() + " type=" + obj.GetObjectType() + "\n")
		}
	}
	return nil
}

// RunPlugins prints plugin statuses by calling MountSpaceContents + WatchState.
// This mirrors exactly what the UI SpacePlugins component does.
func (sa *SpaceArgs) RunPlugins(c *appcli.Context) error {
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	contentsResp, err := spaceSvc.MountSpaceContents(ctx, &s4wave_space.MountSpaceContentsRequest{})
	if err != nil {
		return errors.Wrap(err, "mount space contents")
	}

	contentsClient, err := sa.getResourceClient(ctx, contentsResp.GetResourceId())
	if err != nil {
		return errors.Wrap(err, "space contents client")
	}
	contentsSvc := s4wave_space.NewSRPCSpaceContentsResourceServiceClient(contentsClient)

	strm, err := contentsSvc.WatchState(ctx, &s4wave_space.WatchSpaceContentsStateRequest{})
	if err != nil {
		return errors.Wrap(err, "watch state")
	}
	defer strm.Close()

	state, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv state")
	}

	w := os.Stdout
	w.WriteString("ready: " + strconv.FormatBool(state.GetReady()) + "\n")

	plugins := state.GetPlugins()
	if len(plugins) == 0 {
		w.WriteString("no plugins\n")
		return nil
	}
	w.WriteString("plugins (" + strconv.Itoa(len(plugins)) + "):\n")
	for _, p := range plugins {
		w.WriteString("  - id=" + p.GetPluginId() +
			" approval=" + p.GetApprovalState().String() +
			" loaded=" + strconv.FormatBool(p.GetLoaded()) +
			" desc=" + strconv.Quote(p.GetDescription()) + "\n")
	}
	return nil
}

// resClient is cached by mountSpaceResource for use by getResourceClient.
var cachedResClient *resource_client.Client

// mountSpaceResource navigates the Resource SDK to the SpaceResourceService.
func (sa *SpaceArgs) mountSpaceResource(ctx context.Context) (s4wave_space.SRPCSpaceResourceServiceClient, func(), error) {
	coreClient, err := sa.client.BuildCoreClient()
	if err != nil {
		return nil, nil, errors.Wrap(err, "connect to core plugin")
	}

	resourceSvc := resource.NewSRPCResourceServiceClient(coreClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "resource client")
	}
	cachedResClient = resClient

	rootRef := resClient.AccessRootResource()
	root, err := s4wave_root.NewRoot(resClient, rootRef)
	if err != nil {
		resClient.Release()
		return nil, nil, errors.Wrap(err, "root resource")
	}

	resp, err := root.MountSessionByIdx(ctx, uint32(sa.client.SessionIdx))
	if err != nil {
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		root.Release()
		resClient.Release()
		return nil, nil, errors.Errorf("no session at index %d", sa.client.SessionIdx)
	}

	sessRef := resClient.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(resClient, sessRef)
	if err != nil {
		sessRef.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "session resource")
	}

	soResp, err := sess.MountSharedObject(ctx, sa.SpaceID)
	if err != nil {
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	soRef := resClient.CreateResourceReference(soResp.GetResourceId())
	soClient, err := soRef.GetClient()
	if err != nil {
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "shared object client")
	}

	soSvc := s4wave_sobject.NewSRPCSharedObjectResourceServiceClient(soClient)
	bodyResp, err := soSvc.MountSharedObjectBody(ctx, &s4wave_sobject.MountSharedObjectBodyRequest{})
	if err != nil {
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "mount shared object body")
	}

	bodyRef := resClient.CreateResourceReference(bodyResp.GetResourceId())
	bodyClient, err := bodyRef.GetClient()
	if err != nil {
		bodyRef.Release()
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "space body client")
	}

	spaceSvc := s4wave_space.NewSRPCSpaceResourceServiceClient(bodyClient)

	cleanup := func() {
		bodyRef.Release()
		soRef.Release()
		sess.Release()
		root.Release()
		resClient.Release()
		cachedResClient = nil
	}
	return spaceSvc, cleanup, nil
}

// getResourceClient gets an SRPC client for a resource ID using the cached resource client.
func (sa *SpaceArgs) getResourceClient(ctx context.Context, resourceID uint32) (srpc.Client, error) {
	if cachedResClient == nil {
		return nil, errors.New("resource client not initialized")
	}
	ref := cachedResClient.CreateResourceReference(resourceID)
	return ref.GetClient()
}
