//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	s4wave_apt "github.com/s4wave/spacewave/sdk/apt"
)

var errAptImportDebStoragePending = errors.New("apt import-deb repository storage is not implemented")

// newAptCommand builds the apt command group.
func newAptCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "apt",
		Usage: "manage Apt repositories",
		Subcommands: []*cli.Command{
			newAptImportDebCommand(),
		},
	}
}

func newAptImportDebCommand() *cli.Command {
	return &cli.Command{
		Name:      "import-deb",
		Usage:     "import a local .deb package into an Apt repository",
		ArgsUsage: "<deb-path>",
		Action: func(c *cli.Context) error {
			debPath, err := validateAptImportDebPath(c.Args().Slice())
			if err != nil {
				return err
			}
			if _, err := s4wave_apt.ParseDebPackageFile(debPath); err != nil {
				return err
			}
			return errAptImportDebStoragePending
		},
	}
}

func validateAptImportDebPath(args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("deb path required")
	}
	debPath := filepath.Clean(args[0])
	if filepath.Ext(debPath) != ".deb" {
		return "", errors.Errorf("deb path must end in .deb: %s", debPath)
	}
	info, err := os.Stat(debPath)
	if err != nil {
		return "", errors.Wrap(err, "stat deb path")
	}
	if !info.Mode().IsRegular() {
		return "", errors.Errorf("deb path is not a regular file: %s", debPath)
	}
	return debPath, nil
}
