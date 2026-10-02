//go:build !js

package spacewave_cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	core_session "github.com/s4wave/spacewave/core/session"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"golang.org/x/term"
)

// newLoginFileCommand builds the command for opening a catalog or provider volume.
func newLoginFileCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:      "file",
		Usage:     "open an existing .s4wave Session volume",
		ArgsUsage: "<path>",
		Flags:     []cli.Flag{statePathFlag(&statePath)},
		Action: func(c *cli.Context) error {
			return runLoginFile(c, statePath, c.String("output"), c.Args().First())
		},
	}
}

// runLoginFile registers a file alias after the source daemon verifies its Sessions.
func runLoginFile(c *cli.Context, statePath, outputFormat, path string) error {
	// Resolve the user-selected volume before contacting either daemon.
	var err error
	if path == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("usage: spacewave login file <path-to-.s4wave-volume>")
		}
		path, err = (&promptui.Prompt{Label: "Path to .s4wave file"}).Run()
		if err != nil {
			return errors.Wrap(err, "read Session file path")
		}
	}
	path = strings.TrimSpace(path)
	path, err = filepath.Abs(path)
	if err != nil {
		return errors.Wrap(err, "resolve Session file path")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return errors.Wrap(err, "resolve Session file")
	}

	// Require a regular .s4wave state file.
	if filepath.Ext(path) != ".s4wave" {
		return errors.New("select a .s4wave file from a Spacewave state directory")
	}
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "stat Session file")
	}
	if !info.Mode().IsRegular() {
		return errors.New("Session file must be a regular .s4wave file")
	}
	if filepath.Base(path) != entrypoint_state.Filename {
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), entrypoint_state.Filename)); err != nil {
			return errors.Wrap(err, "find sibling Session catalog state.s4wave")
		}
	}

	// Ask the source daemon to verify the catalog and its provider volumes.
	ctx := c.Context
	sourcePath := filepath.Dir(path)
	source, err := connectDaemonWithAutostart(ctx, sourcePath)
	if err != nil {
		return errors.Wrap(err, "open Session file's state root")
	}
	defer source.close()
	entries, err := usableFileSessions(ctx, source, path)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errors.New("the selected .s4wave volume has no usable Sessions")
	}

	// Save the validated alias in the target daemon.
	target, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer target.close()
	aliasHash := sha256.Sum256([]byte(path))
	aliasID := "file-" + hex.EncodeToString(aliasHash[:8])
	_, err = target.root.UpsertSpaceRootAlias(ctx, &s4wave_root.SpaceRootAliasRecord{
		AliasId:     aliasID,
		DisplayName: filepath.Base(path),
		Kind:        s4wave_root.SpaceRootKind_SpaceRootKind_S4WAVE_FILE,
		OpenMode:    s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING,
		Native:      &s4wave_root.NativeSpaceRootMetadata{Path: path},
	})
	if err != nil {
		return errors.Wrap(err, "add Session file")
	}

	// Render the Sessions and source path for the selected output format.
	if outputFormat == "json" || outputFormat == "yaml" {
		data, err := protojson.MarshalSlice(protojson.MarshalerConfig{}, entries)
		if err != nil {
			return err
		}
		return formatOutput(data, outputFormat)
	}
	os.Stdout.WriteString("Sessions available from " + path + "\n\n")
	rows := [][]string{{"INDEX", "PROVIDER", "ACCOUNT", "SESSION"}}
	for _, entry := range entries {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		rows = append(rows, []string{
			strconv.FormatUint(uint64(entry.GetSessionIndex()), 10),
			ref.GetProviderId(),
			ref.GetProviderAccountId(),
			ref.GetId(),
		})
	}
	writeTable(os.Stdout, "", rows)
	os.Stdout.WriteString("\nSource state path: " + sourcePath + "\n")
	os.Stdout.WriteString("Use with: spacewave session list --state-path <source state path>\n")
	return nil
}

// sessionsFromLoginFile selects all catalog Sessions or the chosen provider account.
func sessionsFromLoginFile(path string, entries []*core_session.SessionListEntry) []*core_session.SessionListEntry {
	if filepath.Base(path) == entrypoint_state.Filename {
		return entries
	}
	filtered := make([]*core_session.SessionListEntry, 0, len(entries))
	for _, entry := range entries {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		filename := strings.ReplaceAll("p/"+ref.GetProviderId()+"/"+ref.GetProviderAccountId(), "/", "_") + ".s4wave"
		if filepath.Base(path) == filename {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// usableFileSessions returns file Sessions that the source daemon can mount.
func usableFileSessions(ctx context.Context, source *sdkClient, path string) ([]*core_session.SessionListEntry, error) {
	// List usable sessions from the login file.
	entries, err := source.root.ListSessions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list Sessions from file")
	}
	entries = sessionsFromLoginFile(path, entries)
	usable := make([]*core_session.SessionListEntry, 0, len(entries))
	var mountErr error
	for _, entry := range entries {
		sess, err := source.mountSession(ctx, entry.GetSessionIndex())
		if err != nil {
			mountErr = err
			continue
		}
		sess.Release()
		usable = append(usable, entry)
	}
	if len(usable) == 0 && mountErr != nil {
		return nil, errors.Wrap(mountErr, "mount Sessions from file")
	}
	return usable, nil
}
