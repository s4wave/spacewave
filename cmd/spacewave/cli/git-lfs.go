//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	git_lfs "github.com/s4wave/spacewave/core/git/lfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// gitLfsAgentName is the git-lfs custom transfer agent name setup configures.
const gitLfsAgentName = "spacewave"

// gitLfsHookMarker identifies a pre-push hook written by setup.
const gitLfsHookMarker = "# spacewave git lfs:"

// buildGitLfsCommand builds the git lfs command group.
func buildGitLfsCommand() *cli.Command {
	return &cli.Command{
		Name:  "lfs",
		Usage: "store Git LFS objects in a Space",
		Subcommands: []*cli.Command{
			buildGitLfsSetupCommand(),
			buildGitLfsAgentCommand(),
			buildGitLfsFlushCommand(),
		},
	}
}

// buildGitLfsSetupCommand builds the git lfs setup subcommand.
func buildGitLfsSetupCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "setup",
		Usage:     "configure this Git repository to store LFS objects in a Space",
		ArgsUsage: "[object-key]",
		Description: "Run inside a Git work tree. Creates the UnixFS object (default\n" +
			"git-lfs/<repository directory>) when missing, installs the local\n" +
			"git-lfs filters, points git-lfs at this command as its transfer\n" +
			"agent, and installs a pre-push hook that waits for the Space to\n" +
			"sync the pushed objects. Rerunning it changes nothing.",
		Flags: commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			return runGitLfsSetup(c, statePath, spaceID, sessIdx)
		},
	}
}

// runGitLfsSetup configures the Git repository in the working directory.
func runGitLfsSetup(c *cli.Context, statePath, spaceID string, sessIdx int) error {
	ctx := c.Context

	// Locate the repository and choose the object key.
	top, err := gitOutput(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return errors.Wrap(err, "find Git work tree")
	}
	hooksDir, err := gitOutput(ctx, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		return errors.Wrap(err, "find Git hooks directory")
	}
	key := c.Args().First()
	if key == "" {
		key = "git-lfs/" + filepath.Base(top)
	}
	if findSubpathDelimiter(key) >= 0 {
		return errors.New("object key cannot contain /-/")
	}
	tracked, err := countGitLfsFiles(ctx)
	if err != nil {
		return err
	}
	idx := uint32(1)
	if sessIdx > 0 {
		if idx, err = sessionIndexFromInt(sessIdx); err != nil {
			return err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "resolve executable")
	}
	exe = filepath.Clean(exe)
	daemonFlags, err := gitLfsDaemonFlags(c, statePath)
	if err != nil {
		return err
	}

	// Resolve the Space and create the object when missing.
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, idx)
	if err != nil {
		return err
	}
	defer sess.Release()
	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		return err
	}
	defer spaceCleanup()
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()
	created, err := ensureGitLfsObject(ctx, engine, key)
	if err != nil {
		return err
	}

	// Install the filters and point git-lfs at the agent. The URI pins the
	// session, Space and object so a later default cannot redirect pushes.
	// Transfers run in one agent: every upload commits to the same object,
	// so concurrent agents only replay each other's commits. The agent
	// serves every remote, so git-lfs skips probing an SSH remote for its
	// own LFS transfer protocol.
	uri := "/u/" + strconv.FormatUint(uint64(idx), 10) + "/so/" + sid + "/-/" + key
	if _, err := gitOutput(ctx, "lfs", "install", "--local", "--skip-repo"); err != nil {
		return errors.Wrap(err, "install git-lfs filters")
	}
	agentArgs := append(append([]string{"git", "lfs", "agent"}, daemonFlags...), uri)
	prefix := "lfs.customtransfer." + gitLfsAgentName + "."
	for _, kv := range [][2]string{
		{"lfs.standalonetransferagent", gitLfsAgentName},
		{prefix + "path", exe},
		{prefix + "args", shellJoin(agentArgs)},
		{prefix + "concurrent", "false"},
		{"lfs.sshtransfer", "never"},
	} {
		if _, err := gitOutput(ctx, "config", "--local", kv[0], kv[1]); err != nil {
			return errors.Wrap(err, "set "+kv[0])
		}
	}

	// Install the pre-push hook that holds the push until the objects sync.
	flushArgs := append(append([]string{exe, "git", "lfs", "flush"}, daemonFlags...), uri)
	hookPath := filepath.Join(hooksDir, "pre-push")
	if err := writeGitLfsHook(hookPath, shellJoin(flushArgs)); err != nil {
		return err
	}

	// Report the result and the next steps.
	w := os.Stdout
	if created {
		w.WriteString("Created UnixFS object " + key + ".\n")
	}
	w.WriteString("Git LFS objects go to " + uri + ".\n\n")
	switch {
	case !created:
		w.WriteString("Next step: git lfs pull\n")
	case tracked != 0:
		// The agent now serves every remote, so the fetch bypasses it to
		// read every version from the old LFS server.
		files := " LFS files"
		if tracked == 1 {
			files = " LFS file"
		}
		w.WriteString("This repository already tracks " + strconv.Itoa(tracked) + files + ". Copy every version into the Space:\n")
		w.WriteString("  git -c lfs.standalonetransferagent= lfs fetch --all origin\n")
		w.WriteString("  git lfs push --all origin\n")
	default:
		w.WriteString("Next steps:\n")
		w.WriteString("  git lfs track '*.png'\n")
		w.WriteString("  git add .gitattributes && git commit -m 'chore: track assets with Git LFS'\n")
		w.WriteString("  git push\n\n")
		w.WriteString("In a fresh clone, run this command, then: git lfs pull\n")
	}
	return nil
}

// buildGitLfsAgentCommand builds the git lfs agent subcommand that git-lfs
// runs as its standalone transfer agent.
func buildGitLfsAgentCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "agent",
		Usage:     "serve git-lfs transfers on stdin and stdout (run by git-lfs)",
		ArgsUsage: "<uri>",
		Hidden:    true,
		Flags:     commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			ctx := c.Context
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}

			// git-lfs renames downloads into its object store, so they
			// must be written on the same filesystem.
			tmpDir, err := gitOutput(ctx, "rev-parse", "--path-format=absolute", "--git-path", "lfs/tmp")
			if err != nil {
				return errors.Wrap(err, "find git-lfs temporary directory")
			}

			fc, cleanup, err := mountFsContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()
			store := git_lfs.NewUnixFSStore(fc.fsSvc, fc.resClient)
			return git_lfs.NewAgent(store, tmpDir).Run(ctx, os.Stdin, os.Stdout)
		},
	}
}

// buildGitLfsFlushCommand builds the git lfs flush subcommand that the
// pre-push hook runs after git-lfs uploads the pushed objects.
func buildGitLfsFlushCommand() *cli.Command {
	var statePath, spaceID string
	var sessIdx int
	return &cli.Command{
		Name:      "flush",
		Usage:     "wait until the session has synced its pending changes",
		ArgsUsage: "<uri>",
		Hidden:    true,
		Flags:     commonFsFlags(&statePath, &spaceID, &sessIdx),
		Action: func(c *cli.Context) error {
			ctx := c.Context
			uri, err := parseFsURI(c.Args().First(), spaceID, sessIdx)
			if err != nil {
				return err
			}
			client, err := connectDaemonFromContext(ctx, c, statePath)
			if err != nil {
				return err
			}
			defer client.close()
			sess, err := client.mountSession(ctx, uri.sessionIdx)
			if err != nil {
				return err
			}
			defer sess.Release()
			return waitSessionSynced(ctx, sess)
		},
	}
}

// waitSessionSynced watches the session sync status until no work is
// pending, reporting progress on stderr, and fails on a sync error.
func waitSessionSynced(ctx context.Context, sess *s4wave_session.Session) error {
	strm, err := sess.WatchSyncStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "watch sync status")
	}
	defer strm.Close()
	var reported bool
	for {
		status, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv sync status")
		}
		switch status.GetState() {
		case s4wave_session.SyncStatusState_SyncStatusState_SYNCED:
			if reported {
				os.Stderr.WriteString("spacewave: synced\n")
			}
			return nil
		case s4wave_session.SyncStatusState_SyncStatusState_ERROR:
			return errors.Errorf("sync failed: %s", status.GetLastError())
		}
		os.Stderr.WriteString(
			"spacewave: syncing " +
				strconv.FormatUint(uint64(status.GetPendingUploadCount()), 10) + " items, " +
				formatSize(status.GetPendingUploadBytes()) + " pending\n",
		)
		reported = true
	}
}

// ensureGitLfsObject creates a UnixFS object at key when none exists and
// reports whether it did. An existing object that is not a stored UnixFS
// filesystem is an error.
func ensureGitLfsObject(ctx context.Context, engine *sdk_engine.SDKEngine, key string) (bool, error) {
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return false, errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	state, found, err := tx.GetObject(ctx, key)
	world.ReleaseObjectState(state)
	if err != nil {
		return false, err
	}
	if found {
		typeID, err := world_types.GetObjectType(ctx, tx, key)
		if err != nil {
			return false, err
		}
		if typeID != unixfs_world.FSNodeTypeID && typeID != unixfs_world.FSObjectTypeID {
			return false, errors.Errorf("object %s has type %q, want a UnixFS filesystem", key, typeID)
		}
		return false, nil
	}

	op := unixfs_world.NewFsInitOp(key, unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now())
	if _, _, err := tx.ApplyWorldOp(ctx, op, ""); err != nil {
		return false, errors.Wrap(err, "apply fs init op")
	}
	if err := tx.Commit(ctx); err != nil {
		return false, errors.Wrap(err, "commit transaction")
	}
	return true, nil
}

// countGitLfsFiles returns the number of LFS files in HEAD. A repository
// without commits has none.
func countGitLfsFiles(ctx context.Context) (int, error) {
	if _, err := gitOutput(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return 0, nil
	}
	out, err := gitOutput(ctx, "lfs", "ls-files", "--name-only")
	if err != nil {
		return 0, errors.Wrap(err, "list LFS files")
	}
	if out == "" {
		return 0, nil
	}
	return strings.Count(out, "\n") + 1, nil
}

// gitLfsDaemonFlags returns the daemon flags the agent and hook repeat so they
// reach the daemon setup used. Unset flags are omitted, leaving the agent the
// same default resolution setup had.
func gitLfsDaemonFlags(c *cli.Context, statePath string) ([]string, error) {
	var flags []string
	if statePathUserSet(c) {
		resolved, err := resolveStatePathFromContext(c, statePath)
		if err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(resolved)
		if err != nil {
			return nil, err
		}
		flags = append(flags, "--state-path", abs)
	}
	if sock := effectiveSocketPath(c, ""); sock != "" {
		abs, err := filepath.Abs(sock)
		if err != nil {
			return nil, err
		}
		flags = append(flags, "--socket-path", abs)
	}
	return flags, nil
}

// writeGitLfsHook writes the pre-push hook that runs git-lfs, then flushCmd.
// It replaces only a missing hook, one it wrote, or the stock git-lfs hook,
// so a user's own hook is never lost.
func writeGitLfsHook(path, flushCmd string) error {
	hook := "#!/bin/sh\n" +
		gitLfsHookMarker + " upload LFS objects, then wait for the Space to sync them.\n" +
		"command -v git-lfs >/dev/null 2>&1 || { echo >&2 \"spacewave: git-lfs was not found on your path.\"; exit 2; }\n" +
		"git lfs pre-push \"$@\" || exit $?\n" +
		"exec " + flushCmd + "\n"

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if bytes.Equal(existing, []byte(hook)) {
		return nil
	}
	if err == nil && !replaceableGitLfsHook(string(existing)) {
		return errors.Errorf(
			"pre-push hook %s already exists; add these lines to it:\n  git lfs pre-push \"$@\" || exit $?\n  %s",
			path,
			flushCmd,
		)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(hook), 0o755) //nolint:gosec // hooks must be executable.
}

// replaceableGitLfsHook reports whether hook was written by setup or is the
// stock pre-push hook git-lfs installs.
func replaceableGitLfsHook(hook string) bool {
	if strings.Contains(hook, gitLfsHookMarker) {
		return true
	}
	for line := range strings.Lines(hook) {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "command -v git-lfs "):
		case strings.HasPrefix(line, "git lfs pre-push "):
		default:
			return false
		}
	}
	return true
}

// gitOutput runs git with args in the working directory and returns its
// trimmed stdout. A failure carries git's stderr.
func gitOutput(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// shellJoin quotes each word for sh and joins them with spaces.
func shellJoin(words []string) string {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
