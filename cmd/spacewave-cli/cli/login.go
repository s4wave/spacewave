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
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"golang.org/x/term"
)

var (
	loginResolveStatePath = resolveStatePathFromContext
	loginConnectDaemon    = connectDaemon
	loginCloseClient      = func(client *sdkClient) { client.close() }
	loginWithPassword     = func(
		ctx context.Context,
		client *sdkClient,
		providerID string,
		username string,
		password string,
	) (*s4wave_provider_spacewave.LoginOrCreateAccountResponse, error) {
		swProv, cleanup, err := client.lookupSpacewaveProvider(ctx, providerID)
		if err != nil {
			return nil, errors.Wrap(err, "lookup spacewave provider")
		}
		defer cleanup()

		resp, err := swProv.LoginOrCreateAccount(ctx, username, password)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	loginWithEntityKey = func(
		ctx context.Context,
		client *sdkClient,
		providerID string,
		pemData []byte,
	) (*session_pb.SessionListEntry, error) {
		swProv, cleanup, err := client.lookupSpacewaveProvider(ctx, providerID)
		if err != nil {
			return nil, errors.Wrap(err, "lookup spacewave provider")
		}
		defer cleanup()

		resp, err := swProv.LoginWithEntityKey(ctx, pemData)
		if err != nil {
			return nil, err
		}
		return resp.GetSessionListEntry(), nil
	}
	loginBrowserHandoff = func(
		ctx context.Context,
		client *sdkClient,
		providerID string,
		req *s4wave_provider_spacewave.StartBrowserHandoffRequest,
	) (*session_pb.SessionListEntry, error) {
		swProv, cleanup, err := client.lookupSpacewaveProvider(ctx, providerID)
		if err != nil {
			return nil, errors.Wrap(err, "lookup spacewave provider")
		}
		defer cleanup()

		resp, err := swProv.StartBrowserHandoff(ctx, req)
		if err != nil {
			return nil, errors.Wrap(err, "start browser handoff")
		}
		return resp.GetSessionListEntry(), nil
	}
)

// newLoginCommand builds the login command.
func newLoginCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	var useBrowser bool
	var pemFile string
	return &cli.Command{
		Name:  "login",
		Usage: "sign up or log in to a Spacewave account",
		Flags: append(clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:    "username",
				Usage:   "account username",
				EnvVars: []string{"SPACEWAVE_USERNAME"},
			},
			&cli.StringFlag{
				Name:    "password",
				Usage:   "account password (prompted if not provided)",
				EnvVars: []string{"SPACEWAVE_PASSWORD"},
			},
			&cli.StringFlag{
				Name:        "pem-file",
				Usage:       "PEM key file for authentication instead of username/password",
				Destination: &pemFile,
			},
			&cli.StringFlag{
				Name:  "provider-id",
				Usage: "provider ID (default: spacewave)",
			},
			&cli.BoolFlag{
				Name:        "browser",
				Usage:       "use browser handoff instead of direct password login",
				Destination: &useBrowser,
			},
		),
		Subcommands: []*cli.Command{
			newLoginLocalCommand(&statePath, &sessionIdx),
			newLoginBrowserCommand(),
		},
		Action: func(c *cli.Context) error {
			if useBrowser && pemFile != "" {
				return errors.New("--browser and --pem-file cannot be used together")
			}
			if useBrowser {
				return runLoginBrowser(c, statePath, c.String("output"))
			}
			return runLogin(c, statePath, c.String("output"), pemFile)
		},
	}
}

// newLoginLocalCommand builds the login local subcommand.
func newLoginLocalCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:  "local",
		Usage: "create a local offline account",
		Action: func(c *cli.Context) error {
			return runAccountCreateLocal(c, *statePath, c.String("output"))
		},
	}
}

// newLoginBrowserCommand builds the login browser subcommand.
func newLoginBrowserCommand() *cli.Command {
	var statePath string
	var username string
	return &cli.Command{
		Name:  "browser",
		Usage: "sign in via the browser handoff flow",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:  "provider-id",
				Usage: "provider ID (default: spacewave)",
			},
			&cli.StringFlag{
				Name:        "username",
				Usage:       "prefill the browser auth flow with this username",
				Destination: &username,
			},
		},
		Action: func(c *cli.Context) error {
			return runLoginBrowser(c, statePath, c.String("output"))
		},
	}
}

// runLogin implements the login command logic.
func runLogin(c *cli.Context, statePath, outputFormat, pemFile string) error {
	providerID := c.String("provider-id")
	if pemFile != "" {
		if c.String("password") != "" {
			return errors.New("--password cannot be used with --pem-file")
		}
		pemData, err := os.ReadFile(pemFile)
		if err != nil {
			return errors.Wrap(err, "read PEM file")
		}
		return runLoginWithEntityKey(c, statePath, outputFormat, providerID, pemData)
	}

	username := c.String("username")
	if username == "" {
		os.Stderr.WriteString("Username: ")
		var buf [256]byte
		n, err := os.Stdin.Read(buf[:])
		if err != nil {
			return errors.Wrap(err, "read username")
		}
		username = string(buf[:n])
		if len(username) > 0 && username[len(username)-1] == '\n' {
			username = username[:len(username)-1]
		}
		if username == "" {
			return errors.New("username required")
		}
	}

	password := c.String("password")
	if password == "" {
		os.Stderr.WriteString("Password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		os.Stderr.WriteString("\n")
		if err != nil {
			return errors.Wrap(err, "read password")
		}
		password = string(pw)
		if password == "" {
			return errors.New("password required")
		}
	}

	ctx := c.Context
	resolved, err := loginResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := loginConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer loginCloseClient(client)

	resp, err := loginWithPassword(ctx, client, providerID, username, password)
	if err != nil {
		if isBrowserAuthRequired(err) {
			cmdName := c.App.Name
			if cmdName == "" {
				cmdName = "spacewave"
			}
			return errors.Wrap(
				err,
				"login requires browser handoff; rerun with `"+cmdName+" login --browser`",
			)
		}
		return errors.Wrap(err, "login")
	}

	entry := resp.GetSessionListEntry()
	if outputFormat == "json" || outputFormat == "yaml" {
		return printSessionListEntry(entry, outputFormat)
	}

	w := os.Stdout
	action := "Logged in."
	if resp.GetIsNewAccount() {
		action = "Account created."
	}
	w.WriteString(action + "\n\n")

	ref := entry.GetSessionRef().GetProviderResourceRef()
	writeFields(w, [][2]string{
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Session", ref.GetId()},
	})
	return nil
}

func runLoginWithEntityKey(
	c *cli.Context,
	statePath, outputFormat, providerID string,
	pemData []byte,
) error {
	ctx := c.Context
	resolved, err := loginResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := loginConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer loginCloseClient(client)

	entry, err := loginWithEntityKey(ctx, client, providerID, pemData)
	if err != nil {
		return errors.Wrap(err, "login with entity key")
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		return printSessionListEntry(entry, outputFormat)
	}

	os.Stdout.WriteString("Logged in with PEM key.\n\n")
	ref := entry.GetSessionRef().GetProviderResourceRef()
	writeFields(os.Stdout, [][2]string{
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Session", ref.GetId()},
	})
	return nil
}

// isBrowserAuthRequired reports whether the error indicates browser auth is required.
func isBrowserAuthRequired(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "browser_auth_required")
}

// runLoginBrowser implements browser-delegated login for CLI clients.
func runLoginBrowser(c *cli.Context, statePath, outputFormat string) error {
	return runLoginBrowserWithStreams(c, statePath, outputFormat, os.Stdout, os.Stderr)
}

// runLoginBrowserWithStreams implements browser-delegated login for CLI clients.
func runLoginBrowserWithStreams(
	c *cli.Context,
	statePath, outputFormat string,
	stdout, stderr io.Writer,
) error {
	ctx := c.Context
	resolved, err := loginResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := loginConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer loginCloseClient(client)

	providerID := c.String("provider-id")
	_, err = stderr.Write([]byte("Opening browser for Spacewave CLI sign-in...\n"))
	if err != nil {
		return errors.Wrap(err, "write status")
	}

	entry, err := loginBrowserHandoff(
		ctx,
		client,
		providerID,
		&s4wave_provider_spacewave.StartBrowserHandoffRequest{
			ClientType: "cli",
			Username:   c.String("username"),
			AuthIntent: "login",
		},
	)
	if err != nil {
		return err
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		return printSessionListEntry(entry, outputFormat)
	}

	_, err = stdout.Write([]byte("Signed in via browser.\n\n"))
	if err != nil {
		return errors.Wrap(err, "write success")
	}

	ref := entry.GetSessionRef().GetProviderResourceRef()
	writeFields(stdout, [][2]string{
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Session", ref.GetId()},
	})
	return nil
}

func runBrowserSignupWithStreams(
	c *cli.Context,
	statePath, outputFormat string,
	stdout, stderr io.Writer,
) error {
	ctx := c.Context
	resolved, err := loginResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := loginConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer loginCloseClient(client)

	providerID := c.String("provider-id")
	username := c.String("username")
	_, err = stderr.Write([]byte("Opening browser for Spacewave CLI sign-up...\n"))
	if err != nil {
		return errors.Wrap(err, "write status")
	}

	entry, err := loginBrowserHandoff(
		ctx,
		client,
		providerID,
		&s4wave_provider_spacewave.StartBrowserHandoffRequest{
			ClientType: "cli",
			Username:   username,
			AuthIntent: "signup",
		},
	)
	if err != nil {
		return err
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		return printSessionListEntry(entry, outputFormat)
	}

	_, err = stdout.Write([]byte("Browser sign-up complete.\n\n"))
	if err != nil {
		return errors.Wrap(err, "write success")
	}

	ref := entry.GetSessionRef().GetProviderResourceRef()
	writeFields(stdout, [][2]string{
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Session", ref.GetId()},
	})
	return nil
}
