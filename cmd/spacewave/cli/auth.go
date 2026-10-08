//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	spacewave_api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	session_pb "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"golang.org/x/term"
)

const (
	localSessionThresholdShowMessage = "auth threshold is not available for local sessions; local sessions manage entity keypairs directly"
	localSessionThresholdSetMessage  = "auth threshold cannot be set for local sessions; local sessions manage entity keypairs directly"
)

type authSessionHandle interface {
	Release()
	GetSessionInfo(context.Context) (*s4wave_session.GetSessionInfoResponse, error)
	GetSession() *s4wave_session.Session
}

// credentialAccount lists the account's entity keypairs and their lock state.
type credentialAccount interface {
	WatchEntityKeypairs(
		context.Context,
		*s4wave_account.WatchEntityKeypairsRequest,
	) (s4wave_account.SRPCAccountResourceService_WatchEntityKeypairsClient, error)
}

type authAccountService interface {
	credentialAccount
	WatchAuthMethods(
		context.Context,
		*s4wave_account.WatchAuthMethodsRequest,
	) (s4wave_account.SRPCAccountResourceService_WatchAuthMethodsClient, error)
}

type authThresholdAccountService interface {
	credentialAccount
	WatchAccountInfo(
		context.Context,
		*s4wave_account.WatchAccountInfoRequest,
	) (s4wave_account.SRPCAccountResourceService_WatchAccountInfoClient, error)
	SetSecurityLevel(
		context.Context,
		*s4wave_account.SetSecurityLevelRequest,
	) (*s4wave_account.SetSecurityLevelResponse, error)
}

type mountedAuthSession struct {
	client  *sdkClient
	session *s4wave_session.Session
}

func (s *mountedAuthSession) Release() {
	s.session.Release()
}

func (s *mountedAuthSession) GetSessionInfo(ctx context.Context) (*s4wave_session.GetSessionInfoResponse, error) {
	return s.session.GetSessionInfo(ctx)
}

func (s *mountedAuthSession) GetSession() *s4wave_session.Session {
	return s.session
}

var (
	authResolveStatePath = resolveStatePathFromContext
	authConnectDaemon    = connectDaemon
	authCloseClient      = func(client *sdkClient) { client.close() }
	authMountSession     = func(ctx context.Context, client *sdkClient, idx uint32) (authSessionHandle, error) {
		sess, err := client.mountSession(ctx, idx)
		if err != nil {
			return nil, err
		}
		return &mountedAuthSession{
			client:  client,
			session: sess,
		}, nil
	}
	authAccessMethodAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authAccountService, func(), error) {
		return client.accessAccount(ctx, providerID, accountID)
	}
	authAccessThresholdAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authThresholdAccountService, func(), error) {
		return client.accessAccount(ctx, providerID, accountID)
	}
)

// newAuthCommand builds the top-level auth command group.
func newAuthCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "auth",
		Usage: "manage authentication, locking, and credentials",
		Subcommands: []*cli.Command{
			newAuthMethodCommand(),
			newAuthPasswdCommand(),
			newAuthLockCommand(),
			newAuthUnlockCommand(),
			newAuthThresholdCommand(),
			newAuthBackupCommand(),
		},
	}
}

// newAuthBackupCommand builds the auth backup command group.
func newAuthBackupCommand() *cli.Command {
	return &cli.Command{
		Name:  "backup",
		Usage: "manage backup keys",
		Subcommands: []*cli.Command{
			newAuthBackupGenerateCommand(),
		},
	}
}

// newAuthBackupGenerateCommand builds the auth backup generate command.
func newAuthBackupGenerateCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var pemFile string
	return &cli.Command{
		Name:  "generate",
		Usage: "generate a backup key and save the PEM file",
		Flags: append(
			clientFlags(&statePath, &sessionIdx),
			pemFileFlag(&pemFile),
			&cli.StringFlag{
				Name:  "output",
				Usage: "path to write the backup PEM key",
				Value: "spacewave-backup.pem",
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthBackupGenerate(c, statePath, sessionIndex32(sessionIdx), pemFile)
		},
	}
}

// runAuthBackupGenerate implements the auth backup generate command.
func runAuthBackupGenerate(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string) error {
	return generateAndSaveBackupKey(c, statePath, sessionIdx, authPemFile, c.String("output"))
}

// generateAndSaveBackupKey performs the shared backup-key flow: prompt
// credential, mount session, access account, call GenerateBackupKey, write
// the PEM to outFile, and print a confirmation line. Used by both the
// auth backup generate and auth method add backup subcommands.
func generateAndSaveBackupKey(c *cli.Context, statePath string, sessionIdx uint32, authPemFile, outFile string) error {
	// Resolve the state path.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon and mount the requested session.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Resolve the account service for the mounted session.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Access the account service for the session's provider account.
	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Collect the credential that authorizes the change.
	cred, err := resolveCredential(ctx, sess, acctSvc, authPemFile)
	if err != nil {
		return err
	}

	// Generate and persist the backup key through the account service.
	resp, err := acctSvc.GenerateBackupKey(ctx, &s4wave_account.GenerateBackupKeyRequest{
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "generate backup key")
	}

	// Write the generated PEM key to the output file.
	if err := os.WriteFile(outFile, resp.GetPemData(), 0o600); err != nil {
		return errors.Wrap(err, "write PEM file")
	}

	// Report the saved file and abbreviated peer identifier.
	pidStr := resp.GetPeerId()
	if len(pidStr) > 16 {
		pidStr = pidStr[:16] + "..."
	}
	os.Stdout.WriteString("backup key saved to " + outFile + " (peer " + pidStr + ")\n")
	return nil
}

// passkeyAuthMethod is the auth method of a passkey entity keypair.
const passkeyAuthMethod = "passkey"

// resolveCredential selects the credential that authorizes an account change,
// in the order the app uses: the PEM file, keypairs already unlocked in the
// daemon, the account password, then a passkey confirmed in the browser. A nil
// credential tells the daemon to sign with its unlocked keypairs.
func resolveCredential(
	ctx context.Context,
	sess *s4wave_session.Session,
	acct credentialAccount,
	pemFile string,
) (*session_pb.EntityCredential, error) {
	// Read and wrap the supplied PEM credential when a file is given.
	if pemFile != "" {
		data, err := os.ReadFile(pemFile)
		if err != nil {
			return nil, errors.Wrap(err, "read PEM file")
		}
		return &session_pb.EntityCredential{
			Credential: &session_pb.EntityCredential_PemPrivateKey{PemPrivateKey: data},
		}, nil
	}

	// Read the account's keypairs; unlocked keypairs sign in the daemon.
	strm, err := acct.WatchEntityKeypairs(ctx, &s4wave_account.WatchEntityKeypairsRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "watch entity keypairs")
	}
	defer strm.Close()
	keypairs, err := strm.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "recv entity keypairs")
	}
	if keypairs.GetUnlockedCount() != 0 {
		return nil, nil
	}

	// Prefer the password and fall back to the first passkey.
	var passkeyPeerID string
	for _, state := range keypairs.GetKeypairs() {
		switch kp := state.GetKeypair(); kp.GetAuthMethod() {
		case auth_password.MethodID:
			pw, err := readSecret("Account password")
			if err != nil {
				return nil, err
			}
			return &session_pb.EntityCredential{
				Credential: &session_pb.EntityCredential_Password{Password: pw},
			}, nil
		case passkeyAuthMethod:
			if passkeyPeerID == "" {
				passkeyPeerID = kp.GetPeerId()
			}
		}
	}
	if passkeyPeerID == "" {
		return nil, errors.New("the account has no password or passkey; pass --pem-file")
	}
	return passkeyCredential(ctx, sess, passkeyPeerID)
}

// passkeyCredential confirms the passkey in the browser and recovers the
// entity key it protects.
func passkeyCredential(ctx context.Context, sess *s4wave_session.Session, peerID string) (*session_pb.EntityCredential, error) {
	// Run the passkey ceremony in the system browser through the daemon.
	sessionClient, err := sess.GetResourceRef().GetClient()
	if err != nil {
		return nil, errors.Wrap(err, "session client")
	}
	svc := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(sessionClient)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	strm, err := svc.StartDesktopPasskeyReauth(ctx, &s4wave_provider_spacewave.StartDesktopPasskeyReauthRequest{PeerId: peerID})
	if err != nil {
		return nil, errors.Wrap(err, "passkey reauth")
	}

	// Show the ceremony URL and wait for the browser's result.
	start, err := strm.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "passkey reauth")
	}
	stop := showBrowserURL(cancel, "confirm with your passkey", start.GetOpenUrl())
	reauth, err := strm.Recv()
	stop()
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("passkey confirmation canceled")
		}
		return nil, errors.Wrap(err, "passkey reauth")
	}

	// Recover the entity PEM, asking for the PIN only when the key has one.
	pem, err := provider_spacewave.RecoverPasskeyEntityPem(reauth, func() (string, error) {
		return readSecret("Passkey PIN")
	})
	if err != nil {
		return nil, err
	}
	return &session_pb.EntityCredential{
		Credential: &session_pb.EntityCredential_PemPrivateKey{PemPrivateKey: pem},
	}, nil
}

// readSecret reads a non-empty secret from the terminal without echo.
func readSecret(label string) (string, error) {
	// Prompt on stderr and read without echo.
	os.Stderr.WriteString(label + ": ")
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return "", errors.Wrapf(err, "read %s", strings.ToLower(label))
	}
	if len(secret) == 0 {
		return "", errors.Errorf("%s must not be empty", label)
	}
	return string(secret), nil
}

// promptNewPassword prompts for a new password with confirmation.
func promptNewPassword(label string) (string, error) {
	// Read the password and its confirmation.
	pw, err := readSecret(label)
	if err != nil {
		return "", err
	}
	confirm, err := readSecret("Retype " + label)
	if err != nil {
		return "", err
	}
	if pw != confirm {
		return "", errors.New("passwords do not match")
	}
	return pw, nil
}

// pemFileFlag returns the common --pem-file flag.
func pemFileFlag(dest *string) cli.Flag {
	return &cli.StringFlag{
		Name:        "pem-file",
		Usage:       "PEM key file that authorizes the change, instead of a password or passkey",
		Destination: dest,
	}
}

// --- account auth method ---

// newAuthMethodCommand builds the auth method command group.
func newAuthMethodCommand() *cli.Command {
	return &cli.Command{
		Name:  "method",
		Usage: "manage auth methods (entity keypairs)",
		Subcommands: []*cli.Command{
			newAuthMethodListCommand(),
			newAuthMethodAddCommand(),
			newAuthMethodRemoveCommand(),
		},
	}
}

// newAuthMethodListCommand builds the auth method list command.
func newAuthMethodListCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "list",
		Usage: "list registered entity keypairs",
		Flags: append(
			clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output format (text/json/yaml)",
				EnvVars: []string{"SPACEWAVE_OUTPUT"},
				Value:   "text",
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthMethodList(c, statePath, c.String("output"), sessionIndex32(sessionIdx))
		},
	}
}

// runAuthMethodList implements the auth method list command.
func runAuthMethodList(c *cli.Context, statePath, outputFormat string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := authConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer authCloseClient(client)

	// Mount the requested session on the daemon connection.
	sess, err := authMountSession(ctx, client, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Read the session info to identify the provider and account.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}

	// Extract the provider and account IDs from the session reference.
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Access the account service for the session's provider account.
	acctSvc, acctCleanup, err := authAccessMethodAccount(ctx, client, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Collect the auth method outputs from the local or account service.
	var methods []*authMethodOutput
	if isLocalAuthSession(info) {
		strm, err := acctSvc.WatchEntityKeypairs(ctx, &s4wave_account.WatchEntityKeypairsRequest{})
		if err != nil {
			return errors.Wrap(err, "watch entity keypairs")
		}
		resp, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv entity keypairs")
		}
		methods = buildLocalAuthMethodOutput(resp.GetKeypairs())
	} else {
		strm, err := acctSvc.WatchAuthMethods(ctx, &s4wave_account.WatchAuthMethodsRequest{})
		if err != nil {
			return errors.Wrap(err, "watch auth methods")
		}
		resp, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv auth methods")
		}
		methods = buildAccountAuthMethodOutput(resp.GetAuthMethods())
	}
	return writeAuthMethodOutput(methods, outputFormat)
}

// newAuthMethodAddCommand builds the auth method add command group.
func newAuthMethodAddCommand() *cli.Command {
	return &cli.Command{
		Name:  "add",
		Usage: "add an authentication method",
		Subcommands: []*cli.Command{
			newAuthMethodAddPasswordCommand(),
			newAuthMethodAddPemCommand(),
			newAuthMethodAddBackupCommand(),
		},
	}
}

// newAuthMethodAddPasswordCommand builds the auth method add password command.
func newAuthMethodAddPasswordCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var pemFile string
	return &cli.Command{
		Name:  "password",
		Usage: "add a new password-derived keypair",
		Flags: append(clientFlags(&statePath, &sessionIdx), pemFileFlag(&pemFile)),
		Action: func(c *cli.Context) error {
			return runAuthMethodAddPassword(c, statePath, sessionIndex32(sessionIdx), pemFile)
		},
	}
}

// runAuthMethodAddPassword implements the auth method add password command.
func runAuthMethodAddPassword(c *cli.Context, statePath string, sessionIdx uint32, pemFile string) error {
	return addAuthMethodFlow(c, statePath, sessionIdx, pemFile,
		func(acctInfo *s4wave_account.WatchAccountInfoResponse) (*session_pb.EntityKeypair, string, error) {
			// Prompt for the new password once the current credential is in hand.
			newPassword, err := promptNewPassword("New password")
			if err != nil {
				return nil, "", err
			}

			// Derive the new password keypair from the entity ID and password.
			_, newPriv, err := auth_password.BuildParametersWithUsernamePassword(acctInfo.GetEntityId(), []byte(newPassword))
			if err != nil {
				return nil, "", errors.Wrap(err, "derive new entity key")
			}
			newPeerID, err := peer.IDFromPrivateKey(newPriv)
			if err != nil {
				return nil, "", errors.Wrap(err, "derive new peer ID")
			}
			kp := &session_pb.EntityKeypair{
				PeerId:     newPeerID.String(),
				AuthMethod: auth_password.MethodID,
			}
			return kp, "password auth method added\n", nil
		})
}

// newAuthMethodAddPemCommand builds the auth method add pem command.
func newAuthMethodAddPemCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var authPemFile string
	return &cli.Command{
		Name:  "pem",
		Usage: "add a PEM backup key as an auth method",
		Flags: append(
			clientFlags(&statePath, &sessionIdx),
			pemFileFlag(&authPemFile),
			&cli.StringFlag{
				Name:     "file",
				Usage:    "path to PEM key file to register",
				Required: true,
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthMethodAddPem(c, statePath, sessionIndex32(sessionIdx), authPemFile)
		},
	}
}

// runAuthMethodAddPem implements the auth method add pem command.
func runAuthMethodAddPem(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string) error {
	// Parse the supplied PEM key and derive its peer ID.
	pemData, err := os.ReadFile(c.String("file"))
	if err != nil {
		return errors.Wrap(err, "read PEM file")
	}
	privKey, err := keypem.ParsePrivKeyPem(pemData)
	if err != nil {
		return errors.Wrap(err, "parse PEM key")
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return errors.Wrap(err, "derive peer ID")
	}

	// Register the key through the shared flow.
	pidStr := peerID.String()
	if len(pidStr) > 16 {
		pidStr = pidStr[:16] + "..."
	}
	return addAuthMethodFlow(c, statePath, sessionIdx, authPemFile,
		func(_ *s4wave_account.WatchAccountInfoResponse) (*session_pb.EntityKeypair, string, error) {
			kp := &session_pb.EntityKeypair{
				PeerId:     peerID.String(),
				AuthMethod: "pem",
			}
			return kp, "pem auth method added (peer " + pidStr + ")\n", nil
		})
}

// addAuthMethodFlow performs the shared connect/mount/access/add-auth-method
// flow. The buildKeypair callback runs after the credential is collected and
// WatchAccountInfo recv, and returns the keypair to register plus the success
// message to write.
func addAuthMethodFlow(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	authPemFile string,
	buildKeypair func(*s4wave_account.WatchAccountInfoResponse) (*session_pb.EntityKeypair, string, error),
) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Read the session info to identify the provider and account.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Access the account service for the session's provider account.
	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Collect the credential that authorizes the change.
	cred, err := resolveCredential(ctx, sess, acctSvc, authPemFile)
	if err != nil {
		return err
	}

	// Receive one account info snapshot from the watch stream.
	infoStrm, err := acctSvc.WatchAccountInfo(ctx, &s4wave_account.WatchAccountInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "watch account info")
	}
	acctInfo, err := infoStrm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv account info")
	}

	// Build the keypair to register with the callback.
	kp, successMsg, err := buildKeypair(acctInfo)
	if err != nil {
		return err
	}

	// Register the keypair as an auth method with the credential.
	_, err = acctSvc.AddAuthMethod(ctx, &s4wave_account.AddAuthMethodRequest{
		Keypair:    kp,
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "add auth method")
	}

	// Print the success message from the callback.
	os.Stdout.WriteString(successMsg)
	return nil
}

// newAuthMethodAddBackupCommand builds the auth method add backup command.
func newAuthMethodAddBackupCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var pemFile string
	return &cli.Command{
		Name:  "backup",
		Usage: "generate a backup key, register it, and save the PEM file",
		Flags: append(
			clientFlags(&statePath, &sessionIdx),
			pemFileFlag(&pemFile),
			&cli.StringFlag{
				Name:  "output-file",
				Usage: "path to write the backup PEM key",
				Value: "spacewave-backup.pem",
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthMethodAddBackup(c, statePath, sessionIndex32(sessionIdx), pemFile)
		},
	}
}

// runAuthMethodAddBackup implements the auth method add backup command.
func runAuthMethodAddBackup(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string) error {
	return generateAndSaveBackupKey(c, statePath, sessionIdx, authPemFile, c.String("output-file"))
}

// newAuthMethodRemoveCommand builds the auth method remove command.
func newAuthMethodRemoveCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var pemFile string
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove an auth method by peer ID",
		ArgsUsage: "<peer-id>",
		Flags:     append(clientFlags(&statePath, &sessionIdx), pemFileFlag(&pemFile)),
		Action: func(c *cli.Context) error {
			pid := c.Args().First()
			if pid == "" {
				return errors.New("peer-id argument required")
			}
			return runAuthMethodRemove(c, statePath, sessionIndex32(sessionIdx), pemFile, pid)
		},
	}
}

// runAuthMethodRemove implements the auth method remove command.
func runAuthMethodRemove(c *cli.Context, statePath string, sessionIdx uint32, authPemFile, peerIDStr string) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Read the session info to identify the provider and account.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Access the account service for the session's provider account.
	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Collect the credential that authorizes the change.
	cred, err := resolveCredential(ctx, sess, acctSvc, authPemFile)
	if err != nil {
		return err
	}

	// Remove the auth method matching the peer ID.
	_, err = acctSvc.RemoveAuthMethod(ctx, &s4wave_account.RemoveAuthMethodRequest{
		PeerId:     peerIDStr,
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "remove auth method")
	}

	// Confirm the removal on stdout.
	os.Stdout.WriteString("auth method removed\n")
	return nil
}

// newAuthMethodReplaceCommand builds the auth method replace command group.
// --- auth passwd ---

// newAuthPasswdCommand builds the auth passwd command.
func newAuthPasswdCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "passwd",
		Usage: "change the account password",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runChangePassword(c, statePath, sessionIndex32(sessionIdx))
		},
	}
}

// runChangePassword implements the password change flow (shared by password set and method replace password).
func runChangePassword(c *cli.Context, statePath string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Prompt for and validate the current password.
	os.Stderr.WriteString("Current password: ")
	oldPw, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return errors.Wrap(err, "read password")
	}
	if len(oldPw) == 0 {
		return errors.New("password must not be empty")
	}

	// Prompt for and confirm the new password.
	newPassword, err := promptNewPassword("New password")
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Read the session info to identify the provider and account.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Access the account service for the session's provider account.
	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Change the account password through the account service.
	_, err = acctSvc.ChangePassword(ctx, &s4wave_account.ChangePasswordRequest{
		OldPassword: string(oldPw),
		NewPassword: newPassword,
	})
	if err != nil {
		return errors.Wrap(err, "change password")
	}

	// Confirm the password change on stdout.
	os.Stdout.WriteString("password changed\n")
	return nil
}

// --- auth lock ---

// newAuthLockCommand builds the auth lock command.
// Bare invocation locks immediately. Subcommands configure mode.
func newAuthLockCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "lock",
		Usage: "lock the session (bare) or configure lock mode (pin, auto, status)",
		Flags: clientFlags(&statePath, &sessionIdx),
		Subcommands: []*cli.Command{
			newAuthLockSetPinCommand(),
			newAuthLockSetAutoCommand(),
			newAuthLockStatusCommand(),
		},
		Action: func(c *cli.Context) error {
			return runAuthLockNow(c, statePath, sessionIndex32(sessionIdx))
		},
	}
}

// newAuthLockSetPinCommand builds the auth lock set pin command.
func newAuthLockSetPinCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "pin",
		Usage: "lock session with a PIN",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runAuthLockSetPin(c, statePath, sessionIndex32(sessionIdx))
		},
	}
}

// runAuthLockSetPin implements the auth lock set pin command.
func runAuthLockSetPin(c *cli.Context, statePath string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Prompt for and confirm the new PIN.
	pin, err := promptNewPassword("PIN")
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Set the session lock mode to PIN-encrypted with the PIN.
	err = sess.SetLockMode(ctx, session_pb.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED, []byte(pin))
	if err != nil {
		return errors.Wrap(err, "set lock mode")
	}

	// Confirm the lock mode change on stdout.
	os.Stdout.WriteString("lock mode set to pin-encrypted\n")
	return nil
}

// newAuthLockSetAutoCommand builds the auth lock set auto command.
func newAuthLockSetAutoCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "auto",
		Usage: "set session to auto-unlock mode",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runAuthLockSetAuto(c, statePath, sessionIndex32(sessionIdx))
		},
	}
}

// runAuthLockSetAuto implements the auth lock set auto command.
func runAuthLockSetAuto(c *cli.Context, statePath string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Set the session lock mode to auto-unlock.
	err = sess.SetLockMode(ctx, session_pb.SessionLockMode_SESSION_LOCK_MODE_AUTO_UNLOCK, nil)
	if err != nil {
		return errors.Wrap(err, "set lock mode")
	}

	// Confirm the lock mode change on stdout.
	os.Stdout.WriteString("lock mode set to auto-unlock\n")
	return nil
}

// runAuthLockNow implements the lock now action.
func runAuthLockNow(c *cli.Context, statePath string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Lock the session immediately.
	err = sess.LockSession(ctx)
	if err != nil {
		return errors.Wrap(err, "lock session")
	}

	// Confirm the lock on stdout.
	os.Stdout.WriteString("session locked\n")
	return nil
}

// newAuthLockStatusCommand builds the auth lock status command.
func newAuthLockStatusCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "status",
		Usage: "show current lock mode and locked state",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runAuthLockStatus(c, statePath, sessionIndex32(sessionIdx))
		},
	}
}

// runAuthLockStatus implements the auth lock status command.
func runAuthLockStatus(c *cli.Context, statePath string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the requested session on the daemon connection.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Receive one lock state snapshot from the watch stream.
	strm, err := sess.WatchLockState(ctx)
	if err != nil {
		return errors.Wrap(err, "watch lock state")
	}
	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "watch lock state")
	}

	// Format the lock mode label from the response.
	mode := "auto-unlock"
	if resp.GetMode() == session_pb.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
		mode = "pin-encrypted"
	}

	// Format the locked label from the response.
	locked := "no"
	if resp.GetLocked() {
		locked = "yes"
	}
	writeFields(os.Stdout, [][2]string{
		{"Mode", mode},
		{"Locked", locked},
	})
	return nil
}

// --- account auth unlock ---

// newAuthUnlockCommand builds the auth unlock command.
func newAuthUnlockCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "unlock",
		Usage: "unlock a PIN-locked session",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runAuthUnlock(c, statePath, sessionIndex32(sessionIdx))
		},
	}
}

// runAuthUnlock implements the auth unlock command.
func runAuthUnlock(c *cli.Context, statePath string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Prompt for the unlock PIN.
	pin, err := readSecret("PIN")
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	// Unlock the session with the PIN.
	err = client.root.UnlockSession(ctx, sessionIdx, []byte(pin))
	if err != nil {
		return errors.Wrap(err, "unlock session")
	}

	// Confirm the unlock on stdout.
	os.Stdout.WriteString("session unlocked\n")
	return nil
}

// --- account auth threshold ---

// newAuthThresholdCommand builds the auth threshold command group.
func newAuthThresholdCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "threshold",
		Usage: "show or set the multi-sig auth threshold",
		Flags: clientFlags(&statePath, &sessionIdx),
		Subcommands: []*cli.Command{
			newAuthThresholdSetCommand(),
		},
		Action: func(c *cli.Context) error {
			return runAuthThresholdShow(c, statePath, sessionIndex32(sessionIdx))
		},
	}
}

// runAuthThresholdShow prints the current auth threshold.
func runAuthThresholdShow(c *cli.Context, statePath string, sessionIdx uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := authConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer authCloseClient(client)

	// Mount the requested session on the daemon connection.
	sess, err := authMountSession(ctx, client, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Read the session info and reject local sessions.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	if isLocalAuthSession(info) {
		return errors.New(localSessionThresholdShowMessage)
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Access the account service for the session's provider account.
	acctSvc, acctCleanup, err := authAccessThresholdAccount(ctx, client, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Receive one account info snapshot from the watch stream.
	strm, err := acctSvc.WatchAccountInfo(ctx, &s4wave_account.WatchAccountInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "watch account info")
	}
	acctInfo, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv account info")
	}

	// Print the auth threshold and keypair count fields.
	writeFields(os.Stdout, [][2]string{
		{"Threshold", strconv.FormatUint(uint64(acctInfo.GetAuthThreshold()), 10)},
		{"Keypairs", strconv.FormatUint(uint64(acctInfo.GetKeypairCount()), 10)},
	})
	return nil
}

// newAuthThresholdSetCommand builds the auth threshold set command.
func newAuthThresholdSetCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var pemFile string
	return &cli.Command{
		Name:      "set",
		Usage:     "set the multi-sig auth threshold",
		ArgsUsage: "<threshold>",
		Flags:     append(clientFlags(&statePath, &sessionIdx), pemFileFlag(&pemFile)),
		Action: func(c *cli.Context) error {
			// Parse the threshold argument for the set action.
			arg := c.Args().First()
			if arg == "" {
				return errors.New("threshold value required as first argument")
			}
			threshold, err := strconv.ParseUint(arg, 10, 32)
			if err != nil {
				return errors.Wrap(err, "parse threshold")
			}
			return runAuthThresholdSet(c, statePath, sessionIndex32(sessionIdx), pemFile, uint32(threshold))
		},
	}
}

// runAuthThresholdSet implements the auth threshold set command.
func runAuthThresholdSet(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string, threshold uint32) error {
	// Resolve the state path from the context or flag.
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Connect to the daemon with the resolved state path.
	client, err := authConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer authCloseClient(client)

	// Mount the requested session on the daemon connection.
	sess, err := authMountSession(ctx, client, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Read the session info and reject local sessions.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	if isLocalAuthSession(info) {
		return errors.New(localSessionThresholdSetMessage)
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	// Access the account service for the session's provider account.
	acctSvc, acctCleanup, err := authAccessThresholdAccount(ctx, client, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Collect the credential that authorizes the change.
	cred, err := resolveCredential(ctx, sess.GetSession(), acctSvc, authPemFile)
	if err != nil {
		return err
	}

	// Set the auth threshold through the account service.
	_, err = acctSvc.SetSecurityLevel(ctx, &s4wave_account.SetSecurityLevelRequest{
		Threshold:  threshold,
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "set security level")
	}

	// Confirm the threshold change on stdout.
	os.Stdout.WriteString("auth threshold set to " + strconv.FormatUint(uint64(threshold), 10) + "\n")
	return nil
}

type authMethodOutput struct {
	PeerID         string
	Label          string
	SecondaryLabel string
	Provider       string
}

func isLocalAuthSession(info *s4wave_session.GetSessionInfoResponse) bool {
	return info.GetSessionRef().GetProviderResourceRef().GetProviderId() == provider_local.ProviderID
}

func buildLocalAuthMethodOutput(states []*s4wave_account.EntityKeypairState) []*authMethodOutput {
	methods := make([]*authMethodOutput, 0, len(states))
	for _, state := range states {
		keypair := state.GetKeypair()
		if keypair == nil {
			continue
		}
		method := keypair.GetAuthMethod()
		label := method
		secondary := ""
		switch method {
		case auth_password.MethodID:
			label = "Password"
		case "pem":
			label = "Backup PEM"
		default:
			if method == "" {
				label = "Unknown"
				break
			}
			secondary = method
		}
		methods = append(methods, &authMethodOutput{
			PeerID:         keypair.GetPeerId(),
			Label:          label,
			SecondaryLabel: secondary,
			Provider:       provider_local.ProviderID,
		})
	}
	return methods
}

func buildAccountAuthMethodOutput(methods []*spacewave_api.AccountAuthMethod) []*authMethodOutput {
	out := make([]*authMethodOutput, 0, len(methods))
	for _, method := range methods {
		if method == nil {
			continue
		}
		out = append(out, &authMethodOutput{
			PeerID:         method.GetPeerId(),
			Label:          method.GetLabel(),
			SecondaryLabel: method.GetSecondaryLabel(),
			Provider:       method.GetProvider(),
		})
	}
	return out
}

func writeAuthMethodOutput(methods []*authMethodOutput, outputFormat string) error {
	// Marshal the auth method outputs as JSON or YAML when requested.
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteArrayStart()
		var af bool
		for _, method := range methods {
			ms.WriteMoreIf(&af)
			ms.WriteObjectStart()
			var f bool
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("peerId")
			ms.WriteString(method.PeerID)
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("label")
			ms.WriteString(method.Label)
			if method.SecondaryLabel != "" {
				ms.WriteMoreIf(&f)
				ms.WriteObjectField("secondaryLabel")
				ms.WriteString(method.SecondaryLabel)
			}
			if method.Provider != "" {
				ms.WriteMoreIf(&f)
				ms.WriteObjectField("provider")
				ms.WriteString(method.Provider)
			}
			ms.WriteObjectEnd()
		}
		ms.WriteArrayEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	// Report an empty auth method list before building table rows.
	if len(methods) == 0 {
		os.Stdout.WriteString("no auth methods\n")
		return nil
	}
	rows := [][]string{{"PEER_ID", "LABEL", "DETAIL"}}
	for _, method := range methods {
		rows = append(rows, []string{
			truncateID(method.PeerID, 20),
			method.Label,
			method.SecondaryLabel,
		})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}
