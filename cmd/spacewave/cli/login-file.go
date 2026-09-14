//go:build !js

package spacewave_cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	core_session "github.com/s4wave/spacewave/core/session"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"golang.org/x/term"
)

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

func runLoginFile(c *cli.Context, statePath, outputFormat, path string) error {
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
	if filepath.Base(path) != "cli.s4wave" {
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), "cli.s4wave")); err != nil {
			return errors.Wrap(err, "find sibling Session catalog cli.s4wave")
		}
	}

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

	target, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer target.close()
	aliasHash := sha256.Sum256([]byte(path))
	aliasID := fmt.Sprintf("file-%x", aliasHash[:8])
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

func sessionsFromLoginFile(path string, entries []*core_session.SessionListEntry) []*core_session.SessionListEntry {
	if filepath.Base(path) == "cli.s4wave" {
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

func usableFileSessions(ctx context.Context, source *sdkClient, path string) ([]*core_session.SessionListEntry, error) {
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
