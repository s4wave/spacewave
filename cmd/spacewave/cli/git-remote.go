//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	git_remote "github.com/s4wave/spacewave/core/git/remote"
)

// gitRemoteHelperName is the executable Git runs for spacewave:// remotes.
const gitRemoteHelperName = "git-remote-spacewave"

// gitRemoteURLPrefix is the URL scheme of a Space Git remote.
const gitRemoteURLPrefix = "spacewave://"

// buildGitRemoteCommand builds the git remote command group.
func buildGitRemoteCommand() *cli.Command {
	return &cli.Command{
		Name:  "remote",
		Usage: "push and fetch Git repositories to a Space with spacewave:// remotes",
		Subcommands: []*cli.Command{
			buildGitRemoteInstallCommand(),
			buildGitRemoteServeCommand(),
		},
	}
}

// buildGitRemoteInstallCommand builds the git remote install subcommand.
func buildGitRemoteInstallCommand() *cli.Command {
	var statePath string
	var dir string
	return &cli.Command{
		Name:  "install",
		Usage: "install " + gitRemoteHelperName + " so Git can use spacewave:// remotes",
		Description: "Writes " + gitRemoteHelperName + " next to this spacewave binary, or into\n" +
			"--dir, which must be on your path. Then add a remote:\n\n" +
			"  git remote add space spacewave://<space>/<object-key>\n" +
			"  git push space master\n\n" +
			"The first push creates the Git repository object. Rerunning install\n" +
			"rewrites the helper for the current binary and daemon flags.",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			socketPathFlag(),
			&cli.StringFlag{
				Name:        "dir",
				Usage:       "directory to write the helper into (default: this binary's directory)",
				Destination: &dir,
			},
		},
		Action: func(c *cli.Context) error {
			exe, err := os.Executable()
			if err != nil {
				return errors.Wrap(err, "find the spacewave binary")
			}
			exe, err = filepath.EvalSymlinks(exe)
			if err != nil {
				return errors.Wrap(err, "find the spacewave binary")
			}
			if dir == "" {
				dir = filepath.Dir(exe)
			}
			daemonFlags, err := gitDaemonFlags(c, statePath)
			if err != nil {
				return err
			}

			serveArgs := append(append([]string{exe, "git", "remote", "serve"}, daemonFlags...), "--")
			script := "#!/bin/sh\n" +
				"# Serves spacewave:// Git remotes. Written by spacewave git remote install.\n" +
				"exec " + shellJoin(serveArgs) + " \"$@\"\n"
			path := filepath.Join(dir, gitRemoteHelperName)
			if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // Git must execute the helper.
				return errors.Wrap(err, "write the remote helper")
			}
			os.Stdout.WriteString("installed " + path + "\n")
			return nil
		},
	}
}

// buildGitRemoteServeCommand builds the git remote serve subcommand that Git
// runs through git-remote-spacewave.
func buildGitRemoteServeCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "serve",
		Usage:     "serve a spacewave:// Git remote on stdin and stdout (run by git)",
		ArgsUsage: "<remote> <url>",
		Hidden:    true,
		Flags:     commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			ctx := c.Context
			space, objectKey, err := parseGitRemoteURL(c.Args().Get(1))
			if err != nil {
				return err
			}
			gitDir, err := gitRemoteDir()
			if err != nil {
				return err
			}

			engine, sess, cleanup, err := mountGitEngine(c, statePath, space, sessIdx)
			if err != nil {
				return err
			}
			defer cleanup()
			helper := git_remote.NewHelper(engine, objectKey, gitDir)
			if err := helper.Run(ctx, os.Stdin, os.Stdout); err != nil {
				return err
			}
			if !helper.Pushed() {
				return nil
			}
			return waitSessionSynced(ctx, sess)
		},
	}
}

// parseGitRemoteURL splits a spacewave://<space>/<object-key> URL. Git passes
// the address without the scheme for the spacewave::<address> form.
func parseGitRemoteURL(url string) (space, objectKey string, err error) {
	addr := strings.TrimPrefix(url, gitRemoteURLPrefix)
	space, objectKey, ok := strings.Cut(addr, "/")
	if !ok || space == "" || objectKey == "" {
		return "", "", errors.Errorf("remote URL %q is not %s<space>/<object-key>", url, gitRemoteURLPrefix)
	}
	return space, objectKey, nil
}

// gitRemoteDir returns the local repository's Git directory, which Git passes
// to remote helpers in GIT_DIR.
func gitRemoteDir() (string, error) {
	dir := os.Getenv("GIT_DIR")
	if dir == "" {
		return "", errors.New("GIT_DIR is not set; git runs this command for spacewave:// remotes")
	}
	return filepath.Abs(dir)
}
