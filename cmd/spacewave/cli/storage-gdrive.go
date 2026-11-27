//go:build !js

package spacewave_cli

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// StorageGdriveArgs connects the selected local account to its Drive folder.
type StorageGdriveArgs struct {
	// statePath selects the daemon that owns the storage credential Secret.
	statePath *string
	// sessionIdx selects the mounted account Session.
	sessionIdx *uint
}

// BuildFlags returns no extra flags; storage supplies the daemon selectors.
func (a *StorageGdriveArgs) BuildFlags() []cli.Flag { return nil }

// newStorageGdriveCommand builds device sign-in for a ciphertext backup folder.
func newStorageGdriveCommand(statePath *string, sessionIdx *uint) *cli.Command {
	args := &StorageGdriveArgs{statePath: statePath, sessionIdx: sessionIdx}
	return &cli.Command{
		Name: "gdrive", Usage: "connect Google Drive for encrypted Space backups", ArgsUsage: "NAME",
		Description: "Prints a URL and code for sign-in on your own device.\n" +
			"Keeps the refresh token only in the account's encrypted credential Secret.\n" +
			"Requires SPACEWAVE_GDRIVE_CLIENT_ID and SPACEWAVE_GDRIVE_CLIENT_SECRET\n" +
			"in both this command's environment and the daemon's environment.",
		Flags: args.BuildFlags(), Action: args.Run,
	}
}

// Run authorizes, creates or reuses the Drive folder, and saves the token in a Secret.
func (a *StorageGdriveArgs) Run(c *cli.Context) error {
	// Validate all local configuration before requesting a device code.
	if c.NArg() != 1 {
		return errors.New("one Drive backend name is required")
	}
	name := c.Args().First()
	conf, err := block_store_gdrive.OAuthConfig()
	if err != nil {
		return err
	}

	return withSession(c, *a.statePath, *a.sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
		// Reauthorization keeps the same backend folder and copy proofs.
		backends, err := readStorageBackends(ctx, sess)
		if err != nil {
			return err
		}
		var location *account_settings.GdriveLocation
		for _, info := range backends.GetStorageBackends() {
			if info.GetBackend().GetDisplayName() == name {
				if info.GetBackend().GetGdrive() == nil {
					return errors.New("that storage name belongs to another backend type")
				}
				location = info.GetBackend().GetGdrive().CloneVT()
			}
		}

		// Display only the verification URL and user code, never a token.
		creds, err := block_store_gdrive.Authorize(ctx, conf, func(uri, code string) error {
			_, err := os.Stdout.WriteString("Open " + uri + "\nEnter code: " + code + "\nWaiting for Google Drive sign-in…\n")
			return err
		})
		if err != nil {
			return err
		}

		// A first connection creates its folder; signing in again reuses it.
		if location == nil {
			httpClient, err := block_store_gdrive.AuthorizedHTTP(ctx, conf, creds)
			if err != nil {
				return err
			}
			client, err := block_store_gdrive.NewClient(httpClient, "root", "")
			if err != nil {
				return err
			}
			folder, err := client.CreateRootFolder(ctx, "Spacewave - "+name)
			if err != nil {
				return err
			}
			location = &account_settings.GdriveLocation{RootFolderId: folder.ID, AccountLabel: name}
		}

		// Save or renew authorization directly in the encrypted credential Secret.
		resp, err := sess.AddStorageBackend(ctx, &s4wave_session.AddStorageBackendRequest{
			DisplayName: name, Gdrive: location, GdriveCredentials: creds,
		})
		if err != nil {
			return err
		}
		if err := checkResultError(resp.GetCheck()); err != nil {
			return err
		}
		_, err = os.Stdout.WriteString("Connected Google Drive: " + name + " (" + resp.GetStorageBackendId() + ")\n")
		return err
	})
}
