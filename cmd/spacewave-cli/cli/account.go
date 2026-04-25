//go:build !js

package spacewave_cli

import (
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// newAccountCommand builds the account command group.
func newAccountCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:    "account",
		Aliases: []string{"accounts"},
		Usage:   "manage accounts",
		Subcommands: []*cli.Command{
			newAccountListCommand(),
			newAccountInfoCommand(),
			newAccountCreateCommand(),
		},
	}
}

// newAccountListCommand builds the account list command.
func newAccountListCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "list",
		Usage: "list accounts grouped by provider",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runAccountList(c, statePath, c.String("output"))
		},
	}
}

// runAccountList implements the account list command logic.
func runAccountList(c *cli.Context, statePath, outputFormat string) error {
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

	// Group sessions by account (provider_id + provider_account_id).
	type acctKey struct{ provider, account string }
	type acctInfo struct {
		provider string
		account  string
		sessions []uint32
	}
	seen := make(map[acctKey]*acctInfo)
	var order []acctKey
	for _, s := range sessions {
		ref := s.GetSessionRef().GetProviderResourceRef()
		k := acctKey{ref.GetProviderId(), ref.GetProviderAccountId()}
		if _, ok := seen[k]; !ok {
			seen[k] = &acctInfo{provider: k.provider, account: k.account}
			order = append(order, k)
		}
		seen[k].sessions = append(seen[k].sessions, s.GetSessionIndex())
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteArrayStart()
		var af bool
		for _, k := range order {
			ms.WriteMoreIf(&af)
			a := seen[k]
			ms.WriteObjectStart()
			var f bool
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("providerId")
			ms.WriteString(a.provider)
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("providerAccountId")
			ms.WriteString(a.account)
			ms.WriteMoreIf(&f)
			ms.WriteObjectField("sessionIndices")
			ms.WriteArrayStart()
			var sf bool
			for _, idx := range a.sessions {
				ms.WriteMoreIf(&sf)
				ms.WriteUint32(idx)
			}
			ms.WriteArrayEnd()
			ms.WriteObjectEnd()
		}
		ms.WriteArrayEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	if len(order) == 0 {
		os.Stdout.WriteString("no accounts\n")
		return nil
	}
	rows := [][]string{{"PROVIDER", "ACCOUNT", "SESSIONS"}}
	for _, k := range order {
		a := seen[k]
		idxStrs := make([]string, len(a.sessions))
		for i, idx := range a.sessions {
			idxStrs[i] = strconv.FormatUint(uint64(idx), 10)
		}
		rows = append(rows, []string{a.provider, a.account, strings.Join(idxStrs, ", ")})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}

// newAccountInfoCommand builds the account info command.
func newAccountInfoCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "info",
		Usage: "show account details",
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
			return runAccountInfo(c, statePath, c.String("output"), uint32(sessionIdx))
		},
	}
}

// runAccountInfo implements the account info command.
func runAccountInfo(c *cli.Context, statePath, outputFormat string, sessionIdx uint32) error {
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

	sessInfo, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	provID := sessInfo.GetSessionRef().GetProviderResourceRef().GetProviderId()
	acctID := sessInfo.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()

	acctSvc, acctCleanup, err := client.accessAccount(ctx, provID, acctID)
	if err != nil {
		return err
	}
	defer acctCleanup()

	strm, err := acctSvc.WatchAccountInfo(ctx, &s4wave_account.WatchAccountInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "watch account info")
	}
	info, err := strm.Recv()
	if err != nil {
		return errors.Wrap(err, "recv account info")
	}

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("accountId")
		ms.WriteString(info.GetAccountId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("entityId")
		ms.WriteString(info.GetEntityId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(info.GetProviderId())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("authThreshold")
		ms.WriteUint32(info.GetAuthThreshold())
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("keypairCount")
		ms.WriteUint32(info.GetKeypairCount())
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	writeFields(os.Stdout, [][2]string{
		{"Account", info.GetAccountId()},
		{"Entity", info.GetEntityId()},
		{"Provider", info.GetProviderId()},
		{"Threshold", strconv.FormatUint(uint64(info.GetAuthThreshold()), 10)},
		{"Keypairs", strconv.FormatUint(uint64(info.GetKeypairCount()), 10)},
	})
	return nil
}

// newAccountCreateCommand builds the account create command.
func newAccountCreateCommand() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create an account and session on a provider",
		Subcommands: []*cli.Command{
			newAccountCreateLocalCommand(),
			newAccountCreateSpacewaveCommand(),
		},
	}
}

// newAccountCreateLocalCommand builds the account create local subcommand.
func newAccountCreateLocalCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "local",
		Usage: "create a local account and session",
		Flags: clientFlags(&statePath, &sessionIdx),
		Action: func(c *cli.Context) error {
			return runAccountCreateLocal(c, statePath, c.String("output"))
		},
	}
}

// runAccountCreateLocal implements the account create local command logic.
func runAccountCreateLocal(c *cli.Context, statePath, outputFormat string) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	localProv, cleanup, err := client.lookupLocalProvider(ctx)
	if err != nil {
		return errors.Wrap(err, "lookup local provider")
	}
	defer cleanup()

	resp, err := localProv.CreateAccount(ctx)
	if err != nil {
		return errors.Wrap(err, "create local provider account")
	}

	return printSessionListEntry(resp.GetSessionListEntry(), outputFormat)
}

// newAccountCreateSpacewaveCommand builds the account create spacewave subcommand.
func newAccountCreateSpacewaveCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "spacewave",
		Usage: "create a spacewave cloud account via browser handoff",
		Flags: append(clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:     "username",
				Usage:    "account username",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "provider-id",
				Usage: "spacewave provider ID (default: spacewave)",
			},
		),
		Action: func(c *cli.Context) error {
			return runAccountCreateSpacewave(c, statePath, c.String("output"))
		},
	}
}

// runAccountCreateSpacewave implements the account create spacewave command logic.
func runAccountCreateSpacewave(c *cli.Context, statePath, outputFormat string) error {
	return runBrowserSignupWithStreams(
		c,
		statePath,
		outputFormat,
		os.Stdout,
		os.Stderr,
	)
}

// printSessionListEntry prints a session list entry in the requested format.
func printSessionListEntry(entry *session_pb.SessionListEntry, outputFormat string) error {
	sessRef := entry.GetSessionRef()
	sessID := sessRef.GetProviderResourceRef().GetId()
	provID := sessRef.GetProviderResourceRef().GetProviderId()
	acctID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	idx := entry.GetSessionIndex()

	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionId")
		ms.WriteString(sessID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerId")
		ms.WriteString(provID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("providerAccountId")
		ms.WriteString(acctID)
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("sessionIndex")
		ms.WriteUint32(idx)
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	w := os.Stdout
	w.WriteString("Account created.\n\n")
	writeFields(w, [][2]string{
		{"Provider", provID},
		{"Account", acctID},
		{"Session", sessID},
		{"Session Index", strconv.FormatUint(uint64(idx), 10)},
	})
	w.WriteString("\nUse --session-index " + strconv.FormatUint(uint64(idx), 10) + " with follow-up commands to use this session.\n")
	return nil
}
