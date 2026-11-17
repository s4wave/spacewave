//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	s4wave_mount "github.com/s4wave/spacewave/sdk/mount"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// pluginApprovalRow is one plugin manifest root as approvals list shows it:
// a root a Space plugin waits on, a decision the session holds, or both.
type pluginApprovalRow struct {
	// pluginID is the plugin the root belongs to.
	pluginID string
	// manifestRoot is the b58 hash the scheduler admits by.
	manifestRoot string
	// state is the decision, or UNKNOWN when the root has none.
	state s4wave_plugin_approval.PluginApprovalState
	// pending is set when a Space plugin waits on the root.
	pending bool
	// peerID is the peer the pending root waits on.
	peerID string
	// rev is the build rev of the pending manifest.
	rev uint64
	// space is the name of the Space whose plugin waits.
	space string
	// since is when the plugin began waiting.
	since time.Time
}

// pluginApprovalsArgs are the arguments every approvals subcommand shares.
type pluginApprovalsArgs struct {
	statePath  string
	sessionIdx uint
}

// BuildFlags returns the flags that select the daemon and session.
func (a *pluginApprovalsArgs) BuildFlags() []cli.Flag {
	return clientFlags(&a.statePath, &a.sessionIdx)
}

// pluginApprovalsSession is a mounted session with its approvals service.
type pluginApprovalsSession struct {
	client  *sdkClient
	sess    *s4wave_session.Session
	service s4wave_session.SRPCSessionResourceServiceClient
	peerID  string
}

// connect mounts the selected session and its SessionResource service.
func (a *pluginApprovalsArgs) connect(c *cli.Context) (*pluginApprovalsSession, error) {
	// Connect to the daemon and mount the selected session.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return nil, err
	}
	sess, err := client.mountSession(ctx, sessionIndex32(a.sessionIdx))
	if err != nil {
		client.close()
		return nil, err
	}
	s := &pluginApprovalsSession{client: client, sess: sess}

	// Read the session peer, which owns the decisions, and open the
	// SessionResource service.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		s.close()
		return nil, errors.Wrap(err, "get session info")
	}
	srpcClient, err := sess.GetResourceRef().GetClient()
	if err != nil {
		s.close()
		return nil, errors.Wrap(err, "session client")
	}
	s.service = s4wave_session.NewSRPCSessionResourceServiceClient(srpcClient)
	s.peerID = info.GetPeerId()
	return s, nil
}

// close releases the session and the daemon connection.
func (s *pluginApprovalsSession) close() {
	s.sess.Release()
	s.client.close()
}

// decisions returns the session's current decisions from the first message
// of WatchPluginApprovals, which holds every decision.
func (s *pluginApprovalsSession) decisions(ctx context.Context) ([]*s4wave_plugin_approval.PluginApproval, error) {
	// Open the decisions watch.
	strm, err := s.service.WatchPluginApprovals(ctx, &s4wave_session.WatchPluginApprovalsRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "watch plugin approvals")
	}
	defer strm.Close()

	// Take the snapshot the watch sends first.
	resp, err := strm.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "recv plugin approvals")
	}
	return resp.GetApprovals(), nil
}

// pending returns the roots a plugin of any Space of the session waits on,
// from the first contents snapshot of each Space.
func (s *pluginApprovalsSession) pending(ctx context.Context) ([]*pluginApprovalRow, error) {
	// List the session's Spaces.
	spaces, err := s4wave_mount.ListSpaces(ctx, s.sess)
	if err != nil {
		return nil, err
	}

	// Collect the waiting roots of each Space's plugins.
	var rows []*pluginApprovalRow
	for _, sp := range spaces {
		name := sp.GetSpaceMeta().GetName()
		plugins, err := s.spacePlugins(ctx, sp.GetEntry().GetRef().GetProviderResourceRef().GetId())
		if err != nil {
			return nil, errors.Wrapf(err, "space %q", name)
		}
		spaceRows, err := pendingApprovalRows(name, plugins)
		if err != nil {
			return nil, errors.Wrapf(err, "space %q", name)
		}
		rows = append(rows, spaceRows...)
	}
	return rows, nil
}

// spacePlugins returns the plugin statuses of one Space's contents snapshot.
func (s *pluginApprovalsSession) spacePlugins(ctx context.Context, spaceID string) ([]*s4wave_space.SpacePluginStatus, error) {
	// Mount the Space and its contents service.
	spaceSvc, spaceCleanup, err := s.client.mountSpace(ctx, s.sess, spaceID)
	if err != nil {
		return nil, err
	}
	defer spaceCleanup()
	contentsSvc, contentsCleanup, err := s.client.mountSpaceContents(ctx, spaceSvc)
	if err != nil {
		return nil, err
	}
	defer contentsCleanup()

	// Take the first contents snapshot, which lists every plugin.
	strm, err := contentsSvc.WatchState(ctx, &s4wave_space.WatchSpaceContentsStateRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "watch state")
	}
	defer strm.Close()
	state, err := strm.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "recv state")
	}
	return state.GetPlugins(), nil
}

// pendingApprovalRows returns a row for each plugin whose swap awaits
// approval, naming the root by the key the scheduler admits it by: the hash
// of the candidate manifest's root ref.
func pendingApprovalRows(space string, plugins []*s4wave_space.SpacePluginStatus) ([]*pluginApprovalRow, error) {
	var rows []*pluginApprovalRow
	for _, p := range plugins {
		// Skip a plugin whose swap does not wait on approval.
		st := p.GetSwap().GetAwaitingApproval()
		if st == nil {
			continue
		}

		// Name the root by the hash of the candidate manifest's root ref.
		ref, err := bucket.ParseObjectRef(st.GetCandidateManifestRef())
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %s candidate manifest ref", p.GetPluginId())
		}
		root := ref.GetRootRef().GetHash().MarshalString()
		if root == "" {
			return nil, errors.Errorf("plugin %s candidate manifest has no root", p.GetPluginId())
		}

		// Record the root with where it waits.
		rows = append(rows, &pluginApprovalRow{
			pluginID:     p.GetPluginId(),
			manifestRoot: root,
			pending:      true,
			peerID:       st.GetPeerId(),
			rev:          st.GetCandidateManifestRev(),
			space:        space,
			since:        st.GetSinceAt().AsTime(),
		})
	}
	return rows, nil
}

// mergeApprovalRows joins pending roots with decisions by (plugin, root),
// sorted by plugin then root. A pending root's decision is never ALLOWED,
// except in the moment between a grant and the scheduler admitting it.
func mergeApprovalRows(pending []*pluginApprovalRow, decisions []*s4wave_plugin_approval.PluginApproval) []*pluginApprovalRow {
	// Keep one row per pending root.
	key := func(pluginID, root string) string { return pluginID + "\x00" + root }
	byKey := make(map[string]*pluginApprovalRow, len(pending)+len(decisions))
	rows := make([]*pluginApprovalRow, 0, len(pending)+len(decisions))
	for _, row := range pending {
		k := key(row.pluginID, row.manifestRoot)
		if byKey[k] == nil {
			byKey[k] = row
			rows = append(rows, row)
		}
	}

	// Join each decision to its pending row, or add a row for it.
	for _, d := range decisions {
		k := key(d.GetPluginId(), d.GetManifestRoot())
		if row := byKey[k]; row != nil {
			row.state = d.GetState()
			continue
		}
		row := &pluginApprovalRow{pluginID: d.GetPluginId(), manifestRoot: d.GetManifestRoot(), state: d.GetState()}
		byKey[k] = row
		rows = append(rows, row)
	}

	// Sort by plugin, then root.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].pluginID != rows[j].pluginID {
			return rows[i].pluginID < rows[j].pluginID
		}
		return rows[i].manifestRoot < rows[j].manifestRoot
	})
	return rows
}

// formatApprovalRow renders one row: plugin, root, state, and for a pending
// root where it waits.
func formatApprovalRow(row *pluginApprovalRow, loc *time.Location) string {
	// Name the decision, or undecided when there is none.
	state := strings.ToLower(strings.TrimPrefix(row.state.String(), "PluginApprovalState_"))
	if row.state == s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_UNKNOWN {
		state = "undecided"
	}

	// Add where a pending root waits.
	fields := []string{row.pluginID, row.manifestRoot, state}
	if row.pending {
		fields = append(fields, "pending rev "+strconv.FormatUint(row.rev, 10)+
			" in "+strconv.Quote(row.space)+
			" since "+row.since.In(loc).Format("15:04:05"))
	}
	return strings.Join(fields, "  ") + "\n"
}

// newPluginApprovalsCommand builds the plugin approvals command group.
func newPluginApprovalsCommand() *cli.Command {
	return &cli.Command{
		Name:  "approvals",
		Usage: "list and approve the plugin manifest roots Space plugins wait on",
		Description: "A Space plugin runs a manifest root only once a release, a development build " +
			"or this device's person admits it. These commands show and record that person's decisions.",
		Subcommands: []*cli.Command{
			newPluginApprovalsListCommand(),
			newPluginApprovalsApproveCommand(),
		},
	}
}

// pluginApprovalsListArgs are the arguments of plugin approvals list.
type pluginApprovalsListArgs struct {
	pluginApprovalsArgs
}

// newPluginApprovalsListCommand builds the plugin approvals list subcommand.
func newPluginApprovalsListCommand() *cli.Command {
	args := &pluginApprovalsListArgs{}
	return &cli.Command{
		Name:   "list",
		Usage:  "list pending roots and the session's decisions: plugin ID, manifest root, state",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// Run prints every pending root and every decision of the session.
func (a *pluginApprovalsListArgs) Run(c *cli.Context) error {
	// Mount the session.
	ctx := c.Context
	s, err := a.connect(c)
	if err != nil {
		return err
	}
	defer s.close()

	// Read the waiting roots and the decisions.
	pending, err := s.pending(ctx)
	if err != nil {
		return err
	}
	decisions, err := s.decisions(ctx)
	if err != nil {
		return err
	}

	// Print one row per root.
	rows := mergeApprovalRows(pending, decisions)
	if len(rows) == 0 {
		os.Stdout.WriteString("no pending roots and no decisions\n")
	}
	for _, row := range rows {
		os.Stdout.WriteString(formatApprovalRow(row, time.Local))
	}
	return nil
}

// pluginApprovalsApproveArgs are the arguments of plugin approvals approve.
type pluginApprovalsApproveArgs struct {
	pluginApprovalsArgs
	allPending bool
}

// newPluginApprovalsApproveCommand builds the plugin approvals approve subcommand.
func newPluginApprovalsApproveCommand() *cli.Command {
	args := &pluginApprovalsApproveArgs{}
	return &cli.Command{
		Name:      "approve",
		Usage:     "allow a plugin manifest root to run on this device",
		ArgsUsage: "<plugin-id> <manifest-root> | --all-pending",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// BuildFlags returns the session flags and --all-pending.
func (a *pluginApprovalsApproveArgs) BuildFlags() []cli.Flag {
	return append(a.pluginApprovalsArgs.BuildFlags(), &cli.BoolFlag{
		Name:        "all-pending",
		Usage:       "allow every root a plugin of the session's Spaces waits on for this session's peer",
		Destination: &a.allPending,
	})
}

// Run records ALLOWED for the named root, or for each pending root.
func (a *pluginApprovalsApproveArgs) Run(c *cli.Context) error {
	// Take either one named root or --all-pending.
	if a.allPending == (c.NArg() != 0) || (!a.allPending && c.NArg() != 2) {
		return errors.New("usage: plugin approvals approve <plugin-id> <manifest-root> | --all-pending")
	}

	// Mount the session.
	ctx := c.Context
	s, err := a.connect(c)
	if err != nil {
		return err
	}
	defer s.close()

	// Approve the named root.
	if !a.allPending {
		return s.approve(ctx, &pluginApprovalRow{pluginID: c.Args().Get(0), manifestRoot: c.Args().Get(1)})
	}

	// Approve each root waiting on this session's peer; a root waiting on
	// another peer is that peer's decision and is only reported.
	pending, err := s.pending(ctx)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		os.Stdout.WriteString("no pending roots\n")
	}

	// Approve each distinct root in order.
	for _, row := range mergeApprovalRows(pending, nil) {
		if row.peerID != s.peerID {
			os.Stdout.WriteString("skipped " + row.pluginID + " " + row.manifestRoot + ": waits on peer " + row.peerID + "\n")
			continue
		}
		if err := s.approve(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

// approve records ALLOWED for one root and prints it.
func (s *pluginApprovalsSession) approve(ctx context.Context, row *pluginApprovalRow) error {
	// Record ALLOWED for the session's peer.
	_, err := s.service.SetPluginApproval(ctx, &s4wave_session.SetPluginApprovalRequest{
		PluginId:     row.pluginID,
		ManifestRoot: row.manifestRoot,
		State:        s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
	})
	if err != nil {
		return errors.Wrapf(err, "approve %s %s", row.pluginID, row.manifestRoot)
	}

	// Report the approved root.
	os.Stdout.WriteString("approved " + row.pluginID + " " + row.manifestRoot + "\n")
	return nil
}
