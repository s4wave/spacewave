//go:build !js

package spacewave_cli

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	tui_host "github.com/s4wave/spacewave/bldr/tui/host"
)

var tuiPluginIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type tuiArgs struct {
	statePath        string
	sessionIndex     uint
	modulePath       string
	exportName       string
	bunPath          string
	spaceName        string
	sessionObjectKey string
	restartLimit     uint
}

func newTuiCommand() *cli.Command {
	args := &tuiArgs{}
	return &cli.Command{
		Name:      "tui",
		Usage:     "open a plugin TuiView against the native Spacewave runtime",
		ArgsUsage: "<plugin-id>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// BuildFlags returns flags for the generic TuiView front door.
func (a *tuiArgs) BuildFlags() []cli.Flag {
	return append(
		clientFlags(&a.statePath, &a.sessionIndex),
		&cli.StringFlag{
			Name:        "module",
			Usage:       "verified focused TuiView module file or file URL",
			EnvVars:     []string{"SPACEWAVE_TUI_MODULE"},
			Required:    true,
			Destination: &a.modulePath,
		},
		&cli.StringFlag{
			Name:        "export",
			Usage:       "TuiView module lifecycle export",
			Value:       "runTuiView",
			Destination: &a.exportName,
		},
		&cli.StringFlag{
			Name:        "bun",
			Usage:       "Bun executable path",
			Value:       "bun",
			Destination: &a.bunPath,
		},
		&cli.StringFlag{
			Name:        "space",
			Usage:       "Space name used by the TuiView",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: &a.spaceName,
		},
		&cli.StringFlag{
			Name:        "llm-session",
			Usage:       "LlmSession object key; remembered Session state is used when empty",
			EnvVars:     []string{"SPACEWAVE_LLM_SESSION"},
			Destination: &a.sessionObjectKey,
		},
		&cli.UintFlag{
			Name:        "restart-limit",
			Usage:       "maximum Bun host restarts after a crash",
			Value:       1,
			Destination: &a.restartLimit,
		},
	)
}

// Run attaches to the daemon and starts the generic Bun TuiView host.
func (a *tuiArgs) Run(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("tui requires one <plugin-id>")
	}
	pluginID := strings.TrimSpace(c.Args().First())
	if !tuiPluginIDPattern.MatchString(pluginID) {
		return errors.Errorf("invalid plugin ID %q", pluginID)
	}
	moduleURL, err := resolveTuiModuleURL(a.modulePath)
	if err != nil {
		return err
	}
	bunPath, err := exec.LookPath(a.bunPath)
	if err != nil {
		return errors.Wrap(err, "resolve Bun executable")
	}
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return err
	}
	client.close()

	daemonSocketPath, err := tuiDaemonSocketPath(c, a.statePath)
	if err != nil {
		return err
	}
	host, err := tui_host.NewHost(tui_host.Config{
		BunPath:          bunPath,
		ModuleURL:        moduleURL,
		ExportName:       a.exportName,
		PluginID:         pluginID,
		DaemonSocketPath: daemonSocketPath,
		SessionIndex:     uint32(a.sessionIndex),
		SessionObjectKey: strings.TrimSpace(a.sessionObjectKey),
		SpaceName:        strings.TrimSpace(a.spaceName),
		StateStoreID:     "tui/" + pluginID,
		RestartLimit:     a.restartLimit,
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
	})
	if err != nil {
		return err
	}
	return host.Run(ctx, nil)
}

func tuiDaemonSocketPath(c *cli.Context, statePath string) (string, error) {
	if socketPath := effectiveSocketPath(c, ""); socketPath != "" {
		if !filepath.IsAbs(socketPath) {
			return "", errors.New("daemon socket path must be absolute")
		}
		return filepath.Clean(socketPath), nil
	}
	resolved, err := resolveStatePathFromContext(c, statePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, socketName), nil
}

func resolveTuiModuleURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("TuiView module is required")
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "file" || !filepath.IsAbs(parsed.Path) {
			return "", errors.New("TuiView module must be an absolute file path or file URL")
		}
		if _, err := os.Stat(parsed.Path); err != nil {
			return "", errors.Wrap(err, "stat TuiView module")
		}
		return parsed.String(), nil
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return "", errors.Wrap(err, "resolve TuiView module")
	}
	if _, err := os.Stat(path); err != nil {
		return "", errors.Wrap(err, "stat TuiView module")
	}
	return (&url.URL{Scheme: "file", Path: path}).String(), nil
}
