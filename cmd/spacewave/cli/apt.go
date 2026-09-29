//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/db/block"
	s4wave_apt "github.com/s4wave/spacewave/sdk/apt"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

type aptImportDebArgs struct {
	statePath  string
	sessionIdx uint
	spaceID    string
}

func (a *aptImportDebArgs) BuildFlags() []cli.Flag {
	return append(clientFlags(&a.statePath, &a.sessionIdx), &cli.StringFlag{
		Name:        "space",
		Aliases:     []string{"space-id"},
		Usage:       "space ID (auto-detected if only one space)",
		EnvVars:     []string{"SPACEWAVE_SPACE"},
		Destination: &a.spaceID,
	})
}

func (a *aptImportDebArgs) Run(c *cli.Context) error {
	if c.NArg() != 3 {
		return errors.New("repository key, package key, and deb path required")
	}
	repositoryKey := c.Args().Get(0)
	packageKey := c.Args().Get(1)
	debPath := c.Args().Get(2)

	// Open the .deb package file for reading.
	debFile, err := os.OpenFile(debPath, os.O_RDONLY|aptDebOpenFlag, 0)
	if err != nil {
		return errors.Wrap(err, "open deb package")
	}
	defer debFile.Close()

	// Require the .deb to be a nonempty regular file within the block size.
	info, err := debFile.Stat()
	if err != nil {
		return errors.Wrap(err, "stat deb package")
	}
	if !info.Mode().IsRegular() {
		return errors.New("deb package must be a regular file")
	}
	if info.Size() == 0 {
		return errors.New("deb package must not be empty")
	}
	if info.Size() > block.MaxBlockSize {
		return errors.Errorf("deb package exceeds maximum block size of %d bytes", block.MaxBlockSize)
	}

	// Read the .deb bytes and recheck the size limits after reading.
	deb, err := io.ReadAll(io.LimitReader(debFile, block.MaxBlockSize+1))
	if err != nil {
		return errors.Wrap(err, "read deb package")
	}
	if len(deb) == 0 {
		return errors.New("deb package must not be empty")
	}
	if len(deb) > block.MaxBlockSize {
		return errors.Errorf("deb package exceeds maximum block size of %d bytes", block.MaxBlockSize)
	}

	// Mount the Space's World engine for the import.
	ctx := c.Context
	engine, cleanup, _, err := mountSpaceWorldEngine(ctx, c, a.statePath, a.sessionIdx, a.spaceID)
	if err != nil {
		return err
	}
	defer cleanup()

	return importAptDebPackage(ctx, engine, repositoryKey, packageKey, deb, os.Stdout)
}

func importAptDebPackage(
	ctx context.Context,
	engine *sdk_engine.SDKEngine,
	repositoryKey string,
	packageKey string,
	deb []byte,
	w io.Writer,
) error {
	// Open a write transaction on the World engine.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	// Import the .deb package and commit the transaction.
	aptPackage, debRef, err := s4wave_apt.ImportDebPackage(ctx, tx, repositoryKey, packageKey, deb)
	if err != nil {
		return errors.Wrap(err, "import deb package")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	// Report the imported package details and the .deb block reference.
	writeFields(w, [][2]string{
		{"Package Key", packageKey},
		{"Package", aptPackage.GetName()},
		{"Version", aptPackage.GetVersion()},
		{"Architecture", aptPackage.GetArchitecture()},
		{"State", strings.TrimPrefix(aptPackage.GetState().String(), "AptPackageState_")},
		{"Deb Ref", debRef.MarshalString()},
	})
	return nil
}

func newAptCommand(getBus func() cli_entrypoint.CliBus) *cli.Command {
	args := &aptImportDebArgs{}
	return &cli.Command{
		Name:  "apt",
		Usage: "manage Apt repositories and packages",
		Subcommands: []*cli.Command{{
			Name:      "import-deb",
			Usage:     "import a Debian package into an Apt repository",
			ArgsUsage: "<repository-key> <package-key> <deb-path>",
			Flags:     args.BuildFlags(),
			Action:    args.Run,
		}},
	}
}
