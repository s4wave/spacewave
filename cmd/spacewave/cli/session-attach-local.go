//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
)

// newSessionAttachLocalCommand builds the command for reconnecting a stored
// local account to the shared Session catalog.
func newSessionAttachLocalCommand() *cli.Command {
	var statePath string
	var list bool
	return &cli.Command{
		Name:        "attach-local",
		Usage:       "attach an existing local account and its Spaces to this state root",
		ArgsUsage:   "<account-id>",
		Description: "Use --list to find unattached local accounts. Example: spacewave session attach-local --state-path ./state <account-id>",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.BoolFlag{Name: "list", Usage: "list local account volumes without a catalog Session", Destination: &list},
		},
		Action: func(c *cli.Context) error {
			if c.Args().Len() > 1 {
				return errors.New("provide one local account ID")
			}
			return runSessionAttachLocal(c, statePath, c.Args().First(), list)
		},
	}
}

// runSessionAttachLocal lists or attaches a local account on the selected
// state root. The daemon owns catalog writes, including when it is already
// running for another renderer.
func runSessionAttachLocal(c *cli.Context, statePath, accountID string, list bool) error {
	// Validate the list flag and account ID arguments.
	if list == (accountID != "") {
		return errors.New("use --list or provide one local account ID")
	}

	// Validate the local account volume before attaching.
	resolved, err := resolveStatePathFromContext(c, statePath)
	if err != nil {
		return err
	}
	if !list {
		if _, err := ulid.ParseULID(accountID); err != nil {
			return errors.Wrap(err, "invalid local account ID")
		}
		if err := checkLocalAccountVolume(resolved, accountID); err != nil {
			return err
		}
	}

	// Connect to the daemon, autostarting it when needed.
	client, err := connectDaemonWithAutostart(c.Context, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Attach the local account through the local provider.
	if list {
		return listUnattachedLocalAccounts(c, client, resolved)
	}
	localProvider, release, err := client.lookupLocalProvider(c.Context)
	if err != nil {
		return err
	}
	defer release()

	// Attach the account and report the resulting Session index.
	resp, err := localProvider.AttachAccount(c.Context, accountID)
	if err != nil {
		return errors.Wrapf(err, "attach local account %s", accountID)
	}
	entry := resp.GetSessionListEntry()
	if entry == nil {
		return errors.New("local provider returned no Session entry")
	}
	os.Stdout.WriteString("local account " + accountID + " has Session index " +
		strconv.FormatUint(uint64(entry.GetSessionIndex()), 10) + "\n")
	return nil
}

// checkLocalAccountVolume proves the target file exists before the daemon can
// mount a local account, since a mount of a missing volume creates one.
func checkLocalAccountVolume(statePath, accountID string) error {
	// Check that the local account volume file exists and is regular.
	path := filepath.Join(statePath, "p_local_"+accountID+".s4wave")
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "find local account volume %s", path)
	}
	if !info.Mode().IsRegular() {
		return errors.Errorf("local account volume %s is not a regular file", path)
	}
	return nil
}

// listUnattachedLocalAccounts compares account-volume files with the daemon's
// current Session catalog.
func listUnattachedLocalAccounts(c *cli.Context, client *sdkClient, statePath string) error {
	// Read the account volume files and the daemon Session catalog.
	entries, err := os.ReadDir(statePath)
	if err != nil {
		return errors.Wrap(err, "read state root")
	}
	sessions, err := client.root.ListSessions(c.Context)
	if err != nil {
		return errors.Wrap(err, "list Sessions")
	}
	attached := make(map[string]struct{})
	for _, entry := range sessions {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		if ref.GetProviderId() == provider_local.ProviderID {
			attached[ref.GetProviderAccountId()] = struct{}{}
		}
	}

	// Collect the unattached account IDs from the volume files.
	var ids []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "p_local_") || !strings.HasSuffix(name, ".s4wave") || !entry.Type().IsRegular() {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "p_local_"), ".s4wave")
		if _, err := ulid.ParseULID(id); err != nil {
			continue
		}
		if _, found := attached[id]; !found {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	if len(ids) == 0 {
		os.Stdout.WriteString("no unattached local accounts\n")
		return nil
	}
	rows := [][]string{{"ACCOUNT ID"}}
	for _, id := range ids {
		rows = append(rows, []string{id})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}
