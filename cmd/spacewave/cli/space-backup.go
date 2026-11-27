//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	s4wave_mount "github.com/s4wave/spacewave/sdk/mount"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// SpaceBackupArgs adapts a command to the device-local backup registry.
type SpaceBackupArgs struct {
	// statePath selects the connected daemon's state path.
	statePath *string
	// sessionIdx selects the mounted account Session.
	sessionIdx *uint
	// backend selects an existing Drive connection for a new backup.
	backend string
	// backup selects one ID when a Space has several backups.
	backup string
	// automatic is an explicit on/off choice, empty to preserve it.
	automatic string
	// interval is the chosen minimum interval between automatic passes.
	interval time.Duration
}

// BuildFlags selects one existing backup for a manual run or schedule change.
func (a *SpaceBackupArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{&cli.StringFlag{Name: "backup", Usage: "backup ID when the Space has several", Destination: &a.backup}}
}

// BuildAddFlags selects an existing Drive connection without enabling a schedule.
func (a *SpaceBackupArgs) BuildAddFlags() []cli.Flag {
	return []cli.Flag{&cli.StringFlag{
		Name: "backend", Required: true, Usage: "existing Drive backend name or ID", Destination: &a.backend,
	}}
}

// BuildSetFlags changes only the explicit approval and interval choices.
func (a *SpaceBackupArgs) BuildSetFlags() []cli.Flag {
	return append(a.BuildFlags(),
		&cli.StringFlag{Name: "auto", Usage: "on or off; enabling starts checkpoint-driven backups", Destination: &a.automatic},
		&cli.DurationFlag{Name: "interval", Usage: "minimum time between automatic passes", Destination: &a.interval},
	)
}

// newSpaceBackupCommand builds the add, run, status and schedule commands.
func newSpaceBackupCommand(statePath *string, sessionIdx *uint) *cli.Command {
	args := &SpaceBackupArgs{statePath: statePath, sessionIdx: sessionIdx}
	return &cli.Command{Name: "backup", Usage: "back up encrypted Space data on this device",
		Subcommands: []*cli.Command{
			{Name: "add", Usage: "configure a Drive backup with automatic work off", ArgsUsage: "SPACE",
				Flags:  args.BuildAddFlags(),
				Action: args.Add},
			{Name: "run", Usage: "run a backup and follow its progress", ArgsUsage: "SPACE",
				Flags: args.BuildFlags(), Action: args.Run},
			{Name: "status", Usage: "show local backup settings and latest results", ArgsUsage: "[SPACE]", Flags: []cli.Flag{outputFlag()}, Action: args.Status},
			{Name: "set", Usage: "choose automatic backups and their minimum interval", ArgsUsage: "SPACE",
				Flags: args.BuildSetFlags(), Action: args.Set},
		}}
}

// Add saves a backup on this device without running or scheduling a pass.
func (a *SpaceBackupArgs) Add(c *cli.Context) error {
	// Resolve the source and an existing Drive credential through the Session.
	if c.NArg() != 1 {
		return errors.New("one Space name or ID is required")
	}
	return withSession(c, *a.statePath, *a.sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
		// Select the source and its already authorized Drive backend.
		id, err := s4wave_mount.ResolveSpaceID(ctx, sess, c.Args().First())
		if err != nil {
			return err
		}
		backend, err := findStorageBackend(ctx, sess, a.backend)
		if err != nil {
			return err
		}
		if backend.GetGdrive() == nil {
			return errors.New("choose a connected Google Drive backend")
		}

		// New configuration is local and explicitly has no automatic work.
		response, err := sess.SetSpaceBackup(ctx, &s4wave_session.SetSpaceBackupRequest{Backup: &account_settings.SpaceBackup{
			SpaceId: id, StorageBackendId: backend.GetId(), IntervalSeconds: 3600,
		}})
		if err != nil {
			return err
		}
		automatic := "off"
		if response.GetBackup().GetAutomatic() {
			automatic = "on"
		}
		_, err = os.Stdout.WriteString("Backup " + response.GetBackup().GetId() + " configured; automatic backups are " + automatic + ".\n")
		return err
	})
}

// Run requests a pass and streams only its public progress and terminal outcome.
func (a *SpaceBackupArgs) Run(c *cli.Context) error {
	// Select one existing backup; the daemon keeps the pass alive after disconnect.
	if c.NArg() != 1 {
		return errors.New("one Space name or ID is required")
	}
	return withSession(c, *a.statePath, *a.sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
		// Request one pass from the account-owned routine and follow its stream.
		backup, err := a.find(ctx, sess, c.Args().First())
		if err != nil {
			return err
		}
		stream, err := sess.RunSpaceBackup(ctx, &s4wave_session.RunSpaceBackupRequest{BackupId: backup.GetId()})
		if err != nil {
			return err
		}
		defer stream.Close()

		// The server ends this watch only after this requested sequence settles.
		for {
			state, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if _, err := os.Stdout.WriteString(backupStateText(state) + "\n"); err != nil {
				return err
			}
			if state.GetCopying() == nil {
				return backupStateError(state)
			}
		}
	})
}

// Status prints public state without opening a storage credential or backup index.
func (a *SpaceBackupArgs) Status(c *cli.Context) error {
	// Read one snapshot from the device-local watch.
	if c.NArg() > 1 {
		return errors.New("at most one Space name or ID is accepted")
	}
	return withSession(c, *a.statePath, *a.sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
		// Read one complete registry snapshot and optionally select a Space.
		list, err := readSpaceBackups(ctx, sess)
		if err != nil {
			return err
		}
		if c.NArg() == 1 {
			id, err := s4wave_mount.ResolveSpaceID(ctx, sess, c.Args().First())
			if err != nil {
				return err
			}
			filtered := &account_settings.SpaceBackupList{}
			for _, state := range list.GetBackups() {
				if state.GetBackup().GetSpaceId() == id {
					filtered.Backups = append(filtered.Backups, state)
				}
			}
			list = filtered
		}
		if format := c.String("output"); format == "json" || format == "yaml" {
			data, err := list.MarshalJSON()
			if err != nil {
				return err
			}
			return formatOutput(data, format)
		}

		// Keep the empty state actionable without a prose tour.
		if len(list.GetBackups()) == 0 {
			_, err := os.Stdout.WriteString("No backups configured on this device. Run space backup add --backend NAME SPACE.\n")
			return err
		}
		rows := [][]string{{"BACKUP", "SPACE", "AUTO", "INTERVAL", "STATE"}}
		for _, state := range list.GetBackups() {
			backup := state.GetBackup()
			auto := "off"
			if backup.GetAutomatic() {
				auto = "on"
			}
			rows = append(rows, []string{backup.GetId(), backup.GetSpaceId(), auto,
				strconv.FormatUint(backup.GetIntervalSeconds(), 10) + "s", backupStateText(state)})
		}
		writeTable(os.Stdout, "", rows)
		return nil
	})
}

// Set changes only explicit schedule fields, preserving the backup's identity.
func (a *SpaceBackupArgs) Set(c *cli.Context) error {
	// Validate the approval-bearing switch before contacting the daemon.
	if c.NArg() != 1 || !c.IsSet("auto") && !c.IsSet("interval") {
		return errors.New("choose one Space and --auto=on|off or --interval=DURATION")
	}
	if c.IsSet("auto") && a.automatic != "on" && a.automatic != "off" {
		return errors.New("--auto must be on or off")
	}
	if c.IsSet("interval") && a.interval < time.Second {
		return errors.New("backup interval must be at least one second")
	}
	return withSession(c, *a.statePath, *a.sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
		// Preserve unselected fields while saving the explicit schedule choices.
		backup, err := a.find(ctx, sess, c.Args().First())
		if err != nil {
			return err
		}
		if c.IsSet("auto") {
			backup.Automatic = a.automatic == "on"
		}
		if seconds := a.interval / time.Second; c.IsSet("interval") && seconds > 0 {
			backup.IntervalSeconds = uint64(seconds)
		}
		_, err = sess.SetSpaceBackup(ctx, &s4wave_session.SetSpaceBackupRequest{Backup: backup})
		return err
	})
}

// find resolves one backup within a Space, requiring an ID for ambiguity.
func (a *SpaceBackupArgs) find(ctx context.Context, sess *s4wave_session.Session, space string) (*account_settings.SpaceBackup, error) {
	// Resolve the Space before inspecting the local backup registry.
	id, err := s4wave_mount.ResolveSpaceID(ctx, sess, space)
	if err != nil {
		return nil, err
	}
	list, err := readSpaceBackups(ctx, sess)
	if err != nil {
		return nil, err
	}
	var selected *account_settings.SpaceBackup
	for _, state := range list.GetBackups() {
		backup := state.GetBackup()
		if backup.GetSpaceId() != id || a.backup != "" && backup.GetId() != a.backup {
			continue
		}
		if selected != nil {
			return nil, errors.New("this Space has several backups; choose --backup=ID")
		}
		selected = backup.CloneVT()
	}
	if selected == nil {
		return nil, errors.New("no backup configured here; run space backup add --backend NAME SPACE")
	}
	return selected, nil
}

// readSpaceBackups reads one watched snapshot and releases the stream.
func readSpaceBackups(ctx context.Context, sess *s4wave_session.Session) (*account_settings.SpaceBackupList, error) {
	// No polling or credential lookup is needed for a current local readback.
	stream, err := sess.WatchSpaceBackups(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return stream.Recv()
}

// backupStateText formats only public counters, names and recovery actions.
func backupStateText(state *account_settings.SpaceBackupState) string {
	switch {
	case state.GetCopying() != nil:
		return "copying: " + strconv.FormatUint(state.GetCopying().GetWrittenBytes(), 10) + " bytes, " + strconv.FormatUint(state.GetCopying().GetPacks(), 10) + " packs"
	case state.GetUpToDate() != nil:
		return "up to date: " + state.GetUpToDate().GetKey()
	case state.GetIncomplete() != nil:
		return "incomplete: " + strconv.FormatUint(state.GetIncomplete().GetHoles(), 10) + " holes; run again after the replica finishes backfilling"
	case state.GetWaitingUploadLimit() != nil:
		return "upload limit reached; retry after " + time.Unix(state.GetWaitingUploadLimit().GetResumeUnixSeconds(), 0).Format(time.RFC3339)
	case state.GetWaitingQuota() != nil:
		return "Drive is full; increase its quota, then run again"
	case state.GetWaitingSignIn() != nil:
		return "Drive authorization expired; sign in again, then run the backup to resume"
	case state.GetFailed() != nil:
		return state.GetFailed().GetMessage()
	default:
		return "idle"
	}
}

// backupStateError gives a command a failing exit status for unfinished work.
func backupStateError(state *account_settings.SpaceBackupState) error {
	if state.GetUpToDate() != nil {
		return nil
	}
	return errors.New(backupStateText(state))
}
