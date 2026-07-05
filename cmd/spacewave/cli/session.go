//go:build !js

package spacewave_cli

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// newSessionCommand builds the session command group.
func newSessionCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:    "session",
		Aliases: []string{"sessions"},
		Usage:   "manage sessions",
		Subcommands: []*cli.Command{
			newSessionListCommand(),
			newSessionInfoCommand(),
			newSessionLogoutCommand(),
			newSessionRevokeCommand(),
		},
	}
}

// newSessionListCommand builds the session list command.
func newSessionListCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "list",
		Usage: "list sessions",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runSessionList(c, statePath, c.String("output"))
		},
	}
}

// runSessionList implements the session list command logic.
func runSessionList(c *cli.Context, statePath, outputFormat string) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	sessions, err := client.root.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		data, err := protojson.MarshalSlice(protojson.MarshalerConfig{}, sessions)
		if err != nil {
			return err
		}
		return formatOutput(data, outputFormat)
	}

	if len(sessions) == 0 {
		os.Stdout.WriteString("no sessions\n")
		return nil
	}
	rows := [][]string{{"INDEX", "SESSION", "PROVIDER", "ACCOUNT"}}
	for _, s := range sessions {
		ref := s.GetSessionRef().GetProviderResourceRef()
		rows = append(rows, []string{
			strconv.FormatUint(uint64(s.GetSessionIndex()), 10),
			truncateID(ref.GetId(), 8),
			ref.GetProviderId(),
			ref.GetProviderAccountId(),
		})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}

// newSessionInfoCommand builds the session info command.
func newSessionInfoCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "info",
		Usage: "show session details and peer info",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runSessionInfo(c, statePath, c.String("output"), uint32(sessionIdx))
		},
	}
}

// runSessionInfo implements the session info command logic.
// This is also called from the hidden "info" alias in cli.go.
func runSessionInfo(c *cli.Context, statePath, outputFormat string, sessionIdx uint32) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
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

	sessID := info.GetSessionRef().GetProviderResourceRef().GetId()
	peerID := info.GetPeerId()
	provID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	strm, err := sess.WatchResourcesList(ctx)
	if err != nil {
		return errors.Wrap(err, "watch resources list")
	}
	defer strm.Close()

	resp, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv resources list")
	}

	spaces := resp.GetSpacesList()

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionId")
		ms.WriteString(sessID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("peerId")
		ms.WriteString(peerID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(provID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerAccountId")
		ms.WriteString(acctID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionIndex")
		ms.WriteUint32(sessionIdx)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("spaceCount")
		ms.WriteInt32(int32(len(spaces)))
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("spaces")
		ms.WriteArrayStart()
		var sf bool
		for _, sp := range spaces {
			ms.WriteMoreIf(&sf)
			ms.WriteObjectStart()
			var spf bool
			ms.WriteMoreIf(&spf)
			ms.WriteObjectField("id")
			ms.WriteString(sp.GetEntry().GetRef().GetProviderResourceRef().GetId())
			ms.WriteMoreIf(&spf)
			ms.WriteObjectField("name")
			ms.WriteString(sp.GetSpaceMeta().GetName())
			ms.WriteObjectEnd()
		}
		ms.WriteArrayEnd()
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	w := os.Stdout
	writeFields(w, [][2]string{
		{"Session", sessID},
		{"Peer", peerID},
		{"Provider", provID},
		{"Account", acctID},
	})
	if len(spaces) > 0 {
		w.WriteString("\nSpaces (" + strconv.Itoa(len(spaces)) + ")\n")
		rows := [][]string{{"ID", "NAME"}}
		for _, sp := range spaces {
			rows = append(rows, []string{
				truncateID(sp.GetEntry().GetRef().GetProviderResourceRef().GetId(), 8),
				sp.GetSpaceMeta().GetName(),
			})
		}
		writeTable(w, "  ", rows)
	}
	return nil
}

// newSessionLogoutCommand builds the session logout subcommand.
func newSessionLogoutCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var yes bool
	var sessionID string
	var accountID string
	return &cli.Command{
		Name:      "logout",
		Aliases:   []string{"signout"},
		Usage:     "sign out a local session",
		ArgsUsage: "[session-index|session-id|account-id]",
		Flags:     sessionLogoutFlags(&statePath, &sessionIdx, &sessionID, &accountID, &yes),
		Action: func(c *cli.Context) error {
			return runSessionLogout(c, statePath, uint32(sessionIdx), sessionLogoutTarget{
				Positional: c.Args().First(),
				SessionID:  sessionID,
				AccountID:  accountID,
			}, yes)
		},
	}
}

type sessionLogoutTarget struct {
	Positional string
	SessionID  string
	AccountID  string
}

func sessionLogoutFlags(statePath *string, sessionIdx *uint, sessionID *string, accountID *string, yes *bool) []cli.Flag {
	return append(
		clientFlags(statePath, sessionIdx),
		&cli.StringFlag{
			Name:        "session-id",
			Usage:       "session id to sign out",
			Destination: sessionID,
		},
		&cli.StringFlag{
			Name:        "account-id",
			Usage:       "provider account id to sign out",
			Destination: accountID,
		},
		&cli.BoolFlag{
			Name:        "yes",
			Aliases:     []string{"y"},
			Usage:       "confirm sign-out without prompting",
			Destination: yes,
		},
	)
}

func runSessionLogout(c *cli.Context, statePath string, sessionIdx uint32, target sessionLogoutTarget, yes bool) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	sessions, err := client.root.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}

	entry, err := resolveSessionLogoutEntry(sessions, target, sessionIdx)
	if err != nil {
		return err
	}

	if !yes {
		ok, err := confirmSessionLogout(entry)
		if err != nil {
			return err
		}
		if !ok {
			os.Stdout.WriteString("sign out canceled\n")
			return nil
		}
	}

	if err := client.root.DeleteSession(ctx, entry.GetSessionIndex()); err != nil {
		return errors.Wrap(err, "delete session")
	}

	ref := entry.GetSessionRef().GetProviderResourceRef()
	os.Stdout.WriteString("signed out session index " + strconv.FormatUint(uint64(entry.GetSessionIndex()), 10) +
		" (" + ref.GetProviderId() + " " + ref.GetProviderAccountId() + ")\n")
	return nil
}

func resolveSessionLogoutEntry(sessions []*core_session.SessionListEntry, target sessionLogoutTarget, sessionIdx uint32) (*core_session.SessionListEntry, error) {
	if len(sessions) == 0 {
		return nil, errors.New("no sessions")
	}
	if target.SessionID != "" && target.AccountID != "" {
		return nil, errors.New("use only one of --session-id or --account-id")
	}
	if target.SessionID != "" {
		return resolveSessionLogoutEntryByProviderRef(sessions, target.SessionID, false)
	}
	if target.AccountID != "" {
		return resolveSessionLogoutEntryByProviderRef(sessions, target.AccountID, true)
	}
	if target.Positional == "" {
		for _, entry := range sessions {
			if entry.GetSessionIndex() == sessionIdx {
				return entry, nil
			}
		}
		return nil, errors.Errorf("no session found at index %d", sessionIdx)
	}

	if idx, err := strconv.ParseUint(target.Positional, 10, 32); err == nil {
		for _, entry := range sessions {
			if entry.GetSessionIndex() == uint32(idx) {
				return entry, nil
			}
		}
		return nil, errors.Errorf("no session found at index %d", idx)
	}

	return resolveSessionLogoutEntryByProviderRef(sessions, target.Positional, true)
}

func resolveSessionLogoutEntryByProviderRef(sessions []*core_session.SessionListEntry, selector string, allowAccount bool) (*core_session.SessionListEntry, error) {
	var match *core_session.SessionListEntry
	for _, entry := range sessions {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		if selector != ref.GetId() && (!allowAccount || selector != ref.GetProviderAccountId()) {
			continue
		}
		if match != nil {
			return nil, errors.Errorf("multiple sessions match %q; use a session index", selector)
		}
		match = entry
	}
	if match != nil {
		return match, nil
	}
	return nil, errors.Errorf("no session found matching %q", selector)
}

func confirmSessionLogout(entry *core_session.SessionListEntry) (bool, error) {
	ref := entry.GetSessionRef().GetProviderResourceRef()
	os.Stdout.WriteString("Sign out this local session?\n\n")
	writeFields(os.Stdout, [][2]string{
		{"Index", strconv.FormatUint(uint64(entry.GetSessionIndex()), 10)},
		{"Session", ref.GetId()},
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
	})
	os.Stdout.WriteString("\nThis removes the session from this Spacewave state root. It does not revoke the provider-side session.\n")
	os.Stdout.WriteString("Continue? [y/N]: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return false, errors.Wrap(err, "read confirmation")
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// newSessionRevokeCommand builds the session revoke command.
func newSessionRevokeCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var pemFile string
	return &cli.Command{
		Name:      "revoke",
		Usage:     "revoke a Spacewave provider session by peer ID",
		ArgsUsage: "<session-peer-id>",
		Flags:     append(clientFlags(&statePath, &sessionIdx), pemFileFlag(&pemFile)),
		Action: func(c *cli.Context) error {
			pid := c.Args().First()
			if pid == "" {
				return errors.New("session peer ID argument required")
			}
			return runSessionRevoke(c, statePath, uint32(sessionIdx), pemFile, pid)
		},
	}
}

// runSessionRevoke implements the session revoke command.
func runSessionRevoke(c *cli.Context, statePath string, sessionIdx uint32, authPemFile, sessionPeerID string) error {
	ctx := c.Context
	resolved, err := resolveStatePathFromContext(c, statePath)
	if err != nil {
		return err
	}
	if err := validateSessionPeerID(sessionPeerID); err != nil {
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

	cred, err := promptCredential(authPemFile)
	if err != nil {
		return err
	}

	_, err = acctSvc.RevokeSession(ctx, &s4wave_account.RevokeSessionRequest{
		SessionPeerId: sessionPeerID,
		Credential:    cred,
	})
	if err != nil {
		return errors.Wrap(err, "revoke session")
	}

	pidStr := sessionPeerID
	if len(pidStr) > 16 {
		pidStr = pidStr[:16] + "..."
	}
	os.Stdout.WriteString("session revoked (" + pidStr + ")\n")
	return nil
}

func validateSessionPeerID(value string) error {
	pid, err := peer.IDB58Decode(value)
	if err != nil || pid.String() != value {
		return errors.Errorf(
			"session revoke requires a session peer ID, got %q; run `spacewave sessions info --session-index <idx>` and use the Peer value",
			value,
		)
	}
	return nil
}
