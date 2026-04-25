//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	spacewave_api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	session_pb "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
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
	AccessLocalSession() (authLocalSessionService, error)
}

type authLocalSessionService interface {
	WatchEntityKeypairs(
		context.Context,
		*s4wave_session.WatchLocalEntityKeypairsRequest,
	) (s4wave_session.SRPCLocalSessionResourceService_WatchEntityKeypairsClient, error)
}

type authMethodAccountService interface {
	WatchAuthMethods(
		context.Context,
		*s4wave_account.WatchAuthMethodsRequest,
	) (s4wave_account.SRPCAccountResourceService_WatchAuthMethodsClient, error)
}

type authThresholdAccountService interface {
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

func (s *mountedAuthSession) AccessLocalSession() (authLocalSessionService, error) {
	return s.client.accessLocalSession(s.session)
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
	authAccessMethodAccount = func(ctx context.Context, client *sdkClient, providerID, accountID string) (authMethodAccountService, func(), error) {
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
		Flags: append(clientFlags(&statePath, &sessionIdx),
			pemFileFlag(&pemFile),
			&cli.StringFlag{
				Name:  "output",
				Usage: "path to write the backup PEM key",
				Value: "spacewave-backup.pem",
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthBackupGenerate(c, statePath, uint32(sessionIdx), pemFile)
		},
	}
}

// runAuthBackupGenerate implements the auth backup generate command.
func runAuthBackupGenerate(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	cred, err := promptCredential(authPemFile)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	resp, err := acctSvc.GenerateBackupKey(ctx, &s4wave_account.GenerateBackupKeyRequest{
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "generate backup key")
	}

	outFile := c.String("output")
	if err := os.WriteFile(outFile, resp.GetPemData(), 0o600); err != nil {
		return errors.Wrap(err, "write PEM file")
	}

	pidStr := resp.GetPeerId()
	if len(pidStr) > 16 {
		pidStr = pidStr[:16] + "..."
	}
	os.Stdout.WriteString("backup key saved to " + outFile + " (peer " + pidStr + ")\n")
	return nil
}

// promptCredential prompts for an entity credential (password or PEM file).
// If pemFile is non-empty, reads the PEM file. Otherwise prompts for password.
func promptCredential(pemFile string) (*session_pb.EntityCredential, error) {
	if pemFile != "" {
		data, err := os.ReadFile(pemFile)
		if err != nil {
			return nil, errors.Wrap(err, "read PEM file")
		}
		return &session_pb.EntityCredential{
			Credential: &session_pb.EntityCredential_PemPrivateKey{PemPrivateKey: data},
		}, nil
	}
	os.Stderr.WriteString("Account password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return nil, errors.Wrap(err, "read password")
	}
	if len(pw) == 0 {
		return nil, errors.New("password must not be empty")
	}
	return &session_pb.EntityCredential{
		Credential: &session_pb.EntityCredential_Password{Password: string(pw)},
	}, nil
}

// promptNewPassword prompts for a new password with confirmation.
func promptNewPassword(label string) (string, error) {
	os.Stderr.WriteString(label + ": ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return "", errors.Wrap(err, "read password")
	}
	os.Stderr.WriteString("Retype " + label + ": ")
	pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return "", errors.Wrap(err, "read password")
	}
	if string(pw) != string(pw2) {
		return "", errors.New("passwords do not match")
	}
	if len(pw) == 0 {
		return "", errors.New("password must not be empty")
	}
	return string(pw), nil
}

// pemFileFlag returns the common --pem-file flag.
func pemFileFlag(dest *string) cli.Flag {
	return &cli.StringFlag{
		Name:        "pem-file",
		Usage:       "PEM key file for authentication (instead of password)",
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
		Flags: append(clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output format (text/json/yaml)",
				EnvVars: []string{"SPACEWAVE_OUTPUT"},
				Value:   "text",
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthMethodList(c, statePath, c.String("output"), uint32(sessionIdx))
		},
	}
}

// runAuthMethodList implements the auth method list command.
func runAuthMethodList(c *cli.Context, statePath, outputFormat string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := authConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer authCloseClient(client)

	sess, err := authMountSession(ctx, client, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}

	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	var methods []*authMethodOutput
	if isLocalAuthSession(info) {
		localSvc, err := sess.AccessLocalSession()
		if err != nil {
			return err
		}
		strm, err := localSvc.WatchEntityKeypairs(ctx, &s4wave_session.WatchLocalEntityKeypairsRequest{})
		if err != nil {
			return errors.Wrap(err, "watch local entity keypairs")
		}
		resp, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "recv local entity keypairs")
		}
		methods = buildLocalAuthMethodOutput(resp.GetKeypairs())
	} else {
		acctSvc, acctCleanup, err := authAccessMethodAccount(ctx, client, provID, acctID)
		if err != nil {
			return err
		}
		defer acctCleanup()

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
			return runAuthMethodAddPassword(c, statePath, uint32(sessionIdx), pemFile)
		},
	}
}

// runAuthMethodAddPassword implements the auth method add password command.
func runAuthMethodAddPassword(c *cli.Context, statePath string, sessionIdx uint32, pemFile string) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	// Prompt for existing credential to authorize.
	cred, err := promptCredential(pemFile)
	if err != nil {
		return err
	}

	// Prompt for new password to add.
	newPassword, err := promptNewPassword("New password")
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	// Get entity ID for key derivation.
	infoStrm, err := acctSvc.WatchAccountInfo(ctx, &s4wave_account.WatchAccountInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "watch account info")
	}
	acctInfo, err := infoStrm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv account info")
	}

	// Derive new keypair from password.
	_, newPriv, err := auth_password.BuildParametersWithUsernamePassword(acctInfo.GetEntityId(), []byte(newPassword))
	if err != nil {
		return errors.Wrap(err, "derive new entity key")
	}
	newPeerID, err := peer.IDFromPrivateKey(newPriv)
	if err != nil {
		return errors.Wrap(err, "derive new peer ID")
	}

	kp := &session_pb.EntityKeypair{
		PeerId:     newPeerID.String(),
		AuthMethod: auth_password.MethodID,
	}

	_, err = acctSvc.AddAuthMethod(ctx, &s4wave_account.AddAuthMethodRequest{
		Keypair:    kp,
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "add auth method")
	}

	os.Stdout.WriteString("password auth method added\n")
	return nil
}

// newAuthMethodAddPemCommand builds the auth method add pem command.
func newAuthMethodAddPemCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var authPemFile string
	return &cli.Command{
		Name:  "pem",
		Usage: "add a PEM backup key as an auth method",
		Flags: append(clientFlags(&statePath, &sessionIdx),
			pemFileFlag(&authPemFile),
			&cli.StringFlag{
				Name:     "file",
				Usage:    "path to PEM key file to register",
				Required: true,
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthMethodAddPem(c, statePath, uint32(sessionIdx), authPemFile)
		},
	}
}

// runAuthMethodAddPem implements the auth method add pem command.
func runAuthMethodAddPem(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	pemFile := c.String("file")
	pemData, err := os.ReadFile(pemFile)
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
	cred, err := promptCredential(authPemFile)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	infoStrm, err := acctSvc.WatchAccountInfo(ctx, &s4wave_account.WatchAccountInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "watch account info")
	}
	_, err = infoStrm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv account info")
	}

	kp := &session_pb.EntityKeypair{
		PeerId:     peerID.String(),
		AuthMethod: "pem",
	}

	_, err = acctSvc.AddAuthMethod(ctx, &s4wave_account.AddAuthMethodRequest{
		Keypair:    kp,
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "add auth method")
	}

	pidStr := peerID.String()
	if len(pidStr) > 16 {
		pidStr = pidStr[:16] + "..."
	}
	os.Stdout.WriteString("pem auth method added (peer " + pidStr + ")\n")
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
		Flags: append(clientFlags(&statePath, &sessionIdx),
			pemFileFlag(&pemFile),
			&cli.StringFlag{
				Name:  "output-file",
				Usage: "path to write the backup PEM key",
				Value: "spacewave-backup.pem",
			},
		),
		Action: func(c *cli.Context) error {
			return runAuthMethodAddBackup(c, statePath, uint32(sessionIdx), pemFile)
		},
	}
}

// runAuthMethodAddBackup implements the auth method add backup command.
func runAuthMethodAddBackup(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	cred, err := promptCredential(authPemFile)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	resp, err := acctSvc.GenerateBackupKey(ctx, &s4wave_account.GenerateBackupKeyRequest{
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "generate backup key")
	}

	outFile := c.String("output-file")
	if err := os.WriteFile(outFile, resp.GetPemData(), 0o600); err != nil {
		return errors.Wrap(err, "write PEM file")
	}

	pidStr := resp.GetPeerId()
	if len(pidStr) > 16 {
		pidStr = pidStr[:16] + "..."
	}
	os.Stdout.WriteString("backup key saved to " + outFile + " (peer " + pidStr + ")\n")
	return nil
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
			return runAuthMethodRemove(c, statePath, uint32(sessionIdx), pemFile, pid)
		},
	}
}

// runAuthMethodRemove implements the auth method remove command.
func runAuthMethodRemove(c *cli.Context, statePath string, sessionIdx uint32, authPemFile, peerIDStr string) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	cred, err := promptCredential(authPemFile)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	_, err = acctSvc.RemoveAuthMethod(ctx, &s4wave_account.RemoveAuthMethodRequest{
		PeerId:     peerIDStr,
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "remove auth method")
	}

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
			return runChangePassword(c, statePath, uint32(sessionIdx))
		},
	}
}

// runChangePassword implements the password change flow (shared by password set and method replace password).
func runChangePassword(c *cli.Context, statePath string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	os.Stderr.WriteString("Current password: ")
	oldPw, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return errors.Wrap(err, "read password")
	}
	if len(oldPw) == 0 {
		return errors.New("password must not be empty")
	}

	newPassword, err := promptNewPassword("New password")
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	_, err = acctSvc.ChangePassword(ctx, &s4wave_account.ChangePasswordRequest{
		OldPassword: string(oldPw),
		NewPassword: newPassword,
	})
	if err != nil {
		return errors.Wrap(err, "change password")
	}

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
			return runAuthLockNow(c, statePath, uint32(sessionIdx))
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
			return runAuthLockSetPin(c, statePath, uint32(sessionIdx))
		},
	}
}

// runAuthLockSetPin implements the auth lock set pin command.
func runAuthLockSetPin(c *cli.Context, statePath string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	pin, err := promptNewPassword("PIN")
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	err = sess.SetLockMode(ctx, session_pb.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED, []byte(pin))
	if err != nil {
		return errors.Wrap(err, "set lock mode")
	}

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
			return runAuthLockSetAuto(c, statePath, uint32(sessionIdx))
		},
	}
}

// runAuthLockSetAuto implements the auth lock set auto command.
func runAuthLockSetAuto(c *cli.Context, statePath string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	err = sess.SetLockMode(ctx, session_pb.SessionLockMode_SESSION_LOCK_MODE_AUTO_UNLOCK, nil)
	if err != nil {
		return errors.Wrap(err, "set lock mode")
	}

	os.Stdout.WriteString("lock mode set to auto-unlock\n")
	return nil
}

// runAuthLockNow implements the lock now action.
func runAuthLockNow(c *cli.Context, statePath string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	err = sess.LockSession(ctx)
	if err != nil {
		return errors.Wrap(err, "lock session")
	}

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
			return runAuthLockStatus(c, statePath, uint32(sessionIdx))
		},
	}
}

// runAuthLockStatus implements the auth lock status command.
func runAuthLockStatus(c *cli.Context, statePath string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	strm, err := sess.WatchLockState(ctx)
	if err != nil {
		return errors.Wrap(err, "watch lock state")
	}
	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "watch lock state")
	}

	mode := "auto-unlock"
	if resp.GetMode() == session_pb.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
		mode = "pin-encrypted"
	}

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
			return runAuthUnlock(c, statePath, uint32(sessionIdx))
		},
	}
}

// runAuthUnlock implements the auth unlock command.
func runAuthUnlock(c *cli.Context, statePath string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	os.Stderr.WriteString("PIN: ")
	pin, err := term.ReadPassword(int(os.Stdin.Fd()))
	os.Stderr.WriteString("\n")
	if err != nil {
		return errors.Wrap(err, "read pin")
	}
	if len(pin) == 0 {
		return errors.New("PIN must not be empty")
	}

	client, err := connectDaemonWithResolvedFallback(ctx, c, resolved)
	if err != nil {
		return err
	}
	defer client.close()

	err = client.root.UnlockSession(ctx, uint32(sessionIdx), pin)
	if err != nil {
		return errors.Wrap(err, "unlock session")
	}

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
			return runAuthThresholdShow(c, statePath, uint32(sessionIdx))
		},
	}
}

// runAuthThresholdShow prints the current auth threshold.
func runAuthThresholdShow(c *cli.Context, statePath string, sessionIdx uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := authConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer authCloseClient(client)

	sess, err := authMountSession(ctx, client, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	if isLocalAuthSession(info) {
		return errors.New(localSessionThresholdShowMessage)
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := authAccessThresholdAccount(ctx, client, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	strm, err := acctSvc.WatchAccountInfo(ctx, &s4wave_account.WatchAccountInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "watch account info")
	}
	acctInfo, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv account info")
	}

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
			arg := c.Args().First()
			if arg == "" {
				return errors.New("threshold value required as first argument")
			}
			threshold, err := strconv.ParseUint(arg, 10, 32)
			if err != nil {
				return errors.Wrap(err, "parse threshold")
			}
			return runAuthThresholdSet(c, statePath, uint32(sessionIdx), pemFile, uint32(threshold))
		},
	}
}

// runAuthThresholdSet implements the auth threshold set command.
func runAuthThresholdSet(c *cli.Context, statePath string, sessionIdx uint32, authPemFile string, threshold uint32) error {
	ctx := c.Context
	resolved, err := authResolveStatePath(c, statePath)
	if err != nil {
		return err
	}

	client, err := authConnectDaemon(ctx, resolved)
	if err != nil {
		return err
	}
	defer authCloseClient(client)

	sess, err := authMountSession(ctx, client, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	if isLocalAuthSession(info) {
		return errors.New(localSessionThresholdSetMessage)
	}
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	cred, err := promptCredential(authPemFile)
	if err != nil {
		return err
	}

	acctSvc, acctCleanup, err := authAccessThresholdAccount(ctx, client, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	_, err = acctSvc.SetSecurityLevel(ctx, &s4wave_account.SetSecurityLevelRequest{
		Threshold:  threshold,
		Credential: cred,
	})
	if err != nil {
		return errors.Wrap(err, "set security level")
	}

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

func buildLocalAuthMethodOutput(keypairs []*session_pb.EntityKeypair) []*authMethodOutput {
	methods := make([]*authMethodOutput, 0, len(keypairs))
	for _, keypair := range keypairs {
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
