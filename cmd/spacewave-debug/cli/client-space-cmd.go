//go:build !js

package cli

import (
	"context"
	"os"
	"strconv"

	appcli "github.com/aperturerobotics/cli"
	"github.com/pkg/errors"

	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// SpaceArgs contains space subcommand arguments.
type SpaceArgs struct {
	client *ClientArgs

	// resClient is the resource client opened by mountSpaceResource and reused
	// by getResourceClient for the duration of one mounted space.
	// The client is cleared when the mount cleanup runs.
	resClient *resource_client.Client

	// SpaceID is the target space ID.
	SpaceID string
	// PluginID is the target plugin manifest ID.
	PluginID string
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
	// Mount the Space resource for this command and release it on return.
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// Add the requested plugin to the Space settings.
	_, err = spaceSvc.AddSpacePlugin(ctx, &s4wave_space.AddSpacePluginRequest{
		PluginId: sa.PluginID,
	})
	if err != nil {
		return errors.Wrap(err, "add space plugin")
	}

	// Report the plugin added to the Space settings.
	os.Stdout.WriteString("added plugin " + sa.PluginID + " to space settings\n")
	return nil
}

// RunRemovePlugin removes a plugin from the space settings.
func (sa *SpaceArgs) RunRemovePlugin(c *appcli.Context) error {
	// Mount the Space resource for this command and release it on return.
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// Remove the requested plugin from the Space settings.
	_, err = spaceSvc.RemoveSpacePlugin(ctx, &s4wave_space.RemoveSpacePluginRequest{
		PluginId: sa.PluginID,
	})
	if err != nil {
		return errors.Wrap(err, "remove space plugin")
	}

	// Report the plugin removed from the Space settings.
	os.Stdout.WriteString("removed plugin " + sa.PluginID + " from space settings\n")
	return nil
}

// RunStatus prints the SpaceState including settings and world contents.
func (sa *SpaceArgs) RunStatus(c *appcli.Context) error {
	// Mount the Space resource for this command and release it on return.
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// Watch the Space state until the first snapshot arrives.
	strm, err := spaceSvc.WatchSpaceState(ctx, &s4wave_space.WatchSpaceStateRequest{})
	if err != nil {
		return errors.Wrap(err, "watch space state")
	}
	defer strm.Close()

	// Receive the current state snapshot for the command output.
	state, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv space state")
	}

	// Print the readiness of the mounted Space.
	w := os.Stdout
	w.WriteString("ready: " + strconv.FormatBool(state.GetReady()) + "\n")

	// Print the configured plugin IDs or the absence of Space settings.
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

	return nil
}

// RunPlugins prints plugin statuses by calling MountSpaceContents + WatchState.
// RunPlugins mirrors exactly what the UI SpacePlugins component does.
func (sa *SpaceArgs) RunPlugins(c *appcli.Context) error {
	// Mount the Space resource for this command and release it on return.
	ctx := c.Context
	spaceSvc, cleanup, err := sa.mountSpaceResource(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// Mount the Space contents to inspect plugin status.
	contentsResp, err := spaceSvc.MountSpaceContents(ctx, &s4wave_space.MountSpaceContentsRequest{})
	if err != nil {
		return errors.Wrap(err, "mount space contents")
	}

	// Open the Space contents service and retain its resource until return.
	contentsClient, releaseContents, err := sa.getResourceClient(ctx, contentsResp.GetResourceId())
	if err != nil {
		return errors.Wrap(err, "space contents client")
	}
	defer releaseContents()
	contentsSvc := s4wave_space.NewSRPCSpaceContentsResourceServiceClient(contentsClient)

	// Watch the Space contents for the first plugin status snapshot.
	strm, err := contentsSvc.WatchState(ctx, &s4wave_space.WatchSpaceContentsStateRequest{})
	if err != nil {
		return errors.Wrap(err, "watch state")
	}
	defer strm.Close()

	// Receive the current state snapshot for the command output.
	state, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv state")
	}

	// Print the readiness of the mounted Space.
	w := os.Stdout
	w.WriteString("ready: " + strconv.FormatBool(state.GetReady()) + "\n")

	// Finish the plugin report when the Space has no plugins.
	plugins := state.GetPlugins()
	if len(plugins) == 0 {
		w.WriteString("no plugins\n")
		return nil
	}

	// Print the loaded state and description of every Space plugin.
	w.WriteString("plugins (" + strconv.Itoa(len(plugins)) + "):\n")
	for _, p := range plugins {
		w.WriteString("  - id=" + p.GetPluginId() +
			" loaded=" + strconv.FormatBool(p.GetLoaded()) +
			" account=" + strconv.FormatBool(p.GetAccount()) +
			" desc=" + strconv.Quote(p.GetDescription()) + "\n")
	}
	return nil
}

// mountSpaceResource mounts the Space SharedObject body in the selected Session
// and returns its Space resource service. Call cleanup when done.
func (sa *SpaceArgs) mountSpaceResource(ctx context.Context) (s4wave_space.SRPCSpaceResourceServiceClient, func(), error) {
	// Mount the selected Session.
	resClient, sess, releaseSess, err := sa.client.mountSession(ctx)
	if err != nil {
		return nil, nil, err
	}
	sa.resClient = resClient

	// Mount the Space SharedObject in the selected Session.
	soResp, err := sess.MountSharedObject(ctx, sa.SpaceID)
	if err != nil {
		releaseSess()
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	// Open the mounted SharedObject client for body access.
	soRef := resClient.CreateResourceReference(soResp.GetResourceId())
	soClient, err := soRef.GetClient()
	if err != nil {
		soRef.Release()
		releaseSess()
		return nil, nil, errors.Wrap(err, "shared object client")
	}

	// Mount the SharedObject body that provides the Space resource.
	soSvc := s4wave_sobject.NewSRPCSharedObjectResourceServiceClient(soClient)
	bodyResp, err := s4wave_sobject.MountSharedObjectBody(ctx, soSvc)
	if err != nil {
		soRef.Release()
		releaseSess()
		return nil, nil, errors.Wrap(err, "mount shared object body")
	}

	// Open the Space body client.
	bodyRef := resClient.CreateResourceReference(bodyResp.GetResourceId())
	bodyClient, err := bodyRef.GetClient()
	if err != nil {
		bodyRef.Release()
		soRef.Release()
		releaseSess()
		return nil, nil, errors.Wrap(err, "space body client")
	}

	// Release the Space body, SharedObject, and Session on cleanup.
	cleanup := func() {
		bodyRef.Release()
		soRef.Release()
		releaseSess()
		sa.resClient = nil
	}
	return s4wave_space.NewSRPCSpaceResourceServiceClient(bodyClient), cleanup, nil
}

// getResourceClient gets an SRPC client for a resource ID using the resource
// client opened by mountSpaceResource.
func (sa *SpaceArgs) getResourceClient(ctx context.Context, resourceID uint32) (srpc.Client, func(), error) {
	// Require the Resource client retained by the mounted Space.
	if sa.resClient == nil {
		return nil, nil, errors.New("resource client not initialized")
	}

	// Acquire the requested resource client and release its reference on failure.
	ref := sa.resClient.CreateResourceReference(resourceID)
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, nil, err
	}
	return client, ref.Release, nil
}
