//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"golang.org/x/term"
)

func newLoginPairCommand() *cli.Command {
	var statePath string
	return &cli.Command{
		Name:    "p2p",
		Aliases: []string{"pair"},
		Usage:   "add an account using a code from another device",
		Flags:   []cli.Flag{statePathFlag(&statePath)},
		Action: func(c *cli.Context) error {
			return runLoginPair(c, statePath, c.String("output"))
		},
	}
}

func runLoginPair(c *cli.Context, statePath, outputFormat string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("device-code pairing needs an interactive terminal")
	}
	code, err := (&promptui.Prompt{Label: "Code from your other device"}).Run()
	if err != nil {
		return errors.Wrap(err, "read pairing code")
	}
	code = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if len(code) != 8 {
		return errors.New("pairing code must have 8 characters")
	}

	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	local, releaseLocal, err := client.lookupLocalProvider(ctx)
	if err != nil {
		return err
	}
	defer releaseLocal()
	prepared, err := local.PreparePairingSession(ctx)
	if err != nil {
		return errors.Wrap(err, "prepare pairing session")
	}
	resourceID, err := client.root.MountSession(ctx, prepared.GetSessionRef())
	if err != nil {
		return errors.Wrap(err, "mount pairing session")
	}
	resourceRef := client.resClient.CreateResourceReference(resourceID)
	sess, err := s4wave_session.NewSession(client.resClient, resourceRef)
	if err != nil {
		resourceRef.Release()
		return errors.Wrap(err, "open pairing session")
	}
	defer sess.Release()

	connected, err := sess.CompletePairing(ctx, code, false)
	if err != nil {
		return errors.Wrap(err, "connect to other device")
	}
	remotePeerID := connected.GetRemotePeerId()
	if remotePeerID == "" {
		return errors.New("pairing did not connect to the other device")
	}
	if err := verifyLoginPairing(ctx, sess, remotePeerID); err != nil {
		return err
	}
	result, err := sess.ConfirmPairingWithResult(ctx, remotePeerID, "")
	if err != nil {
		return errors.Wrap(err, "open paired account")
	}
	entry := result.GetSessionListEntry()
	if entry == nil {
		return errors.New("pairing finished without a registered session")
	}
	if outputFormat == "json" || outputFormat == "yaml" {
		return printSessionListEntry(entry, outputFormat)
	}
	os.Stdout.WriteString("Account connected.\n\n")
	ref := entry.GetSessionRef().GetProviderResourceRef()
	writeFields(os.Stdout, [][2]string{
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Session", ref.GetId()},
	})
	return nil
}

func verifyLoginPairing(ctx context.Context, sess *s4wave_session.Session, remotePeerID string) error {
	watch, err := sess.WatchPairingStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "watch pairing")
	}
	defer watch.Close()
	confirmed := false
	for {
		state, err := watch.Recv()
		if err != nil {
			return errors.Wrap(err, "watch pairing")
		}
		switch state.GetStatus() {
		case s4wave_session.PairingStatus_PairingStatus_SELECTING_ACCOUNT:
			if err := sess.SelectPairingAccount(ctx, pairing.AccountOutcome_AccountOutcome_SIGN_IN_OFFERED); err != nil {
				return errors.Wrap(err, "select offered account")
			}
		case s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI:
			if confirmed {
				continue
			}
			sas, err := sess.GetSASEmoji(ctx, remotePeerID)
			if err != nil {
				return errors.Wrap(err, "read pairing emoji")
			}
			if len(sas.GetEmoji()) != 6 {
				return errors.New("pairing did not provide six verification emoji")
			}
			os.Stdout.WriteString("Confirm these emoji match on both devices:\n")
			for _, emoji := range sas.GetEmoji() {
				os.Stdout.WriteString(emoji + " ")
			}
			os.Stdout.WriteString("\n")
			_, err = (&promptui.Prompt{Label: "Do they match", IsConfirm: true}).Run()
			if err != nil {
				_ = sess.ConfirmSASMatch(ctx, false)
				return errors.New("pairing cancelled")
			}
			if err := sess.ConfirmSASMatch(ctx, true); err != nil {
				return errors.Wrap(err, "confirm emoji")
			}
			confirmed = true
			os.Stdout.WriteString("Waiting for the other device to confirm...\n")
		case s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED:
			if !confirmed {
				return errors.New("other device confirmed before local verification")
			}
			return nil
		case s4wave_session.PairingStatus_PairingStatus_FAILED,
			s4wave_session.PairingStatus_PairingStatus_SIGNALING_FAILED,
			s4wave_session.PairingStatus_PairingStatus_CONNECTION_TIMEOUT,
			s4wave_session.PairingStatus_PairingStatus_PAIRING_REJECTED,
			s4wave_session.PairingStatus_PairingStatus_CONFIRMATION_TIMEOUT:
			return errors.Errorf("pairing failed: %s", state.GetErrorMessage())
		}
	}
}
