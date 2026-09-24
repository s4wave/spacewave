//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"golang.org/x/term"
)

func newLoginPairCommand() *cli.Command {
	var statePath, code, label string
	return &cli.Command{
		Name:    "p2p",
		Aliases: []string{"pair"},
		Usage:   "add an account using a code from another device",
		Flags: []cli.Flag{
			statePathFlag(&statePath),
			&cli.StringFlag{
				Name:        "code",
				Usage:       "pairing code from the other device; skips the interactive prompts",
				EnvVars:     []string{"SPACEWAVE_PAIRING_CODE"},
				Destination: &code,
			},
			&cli.StringFlag{
				Name:        "label",
				Usage:       "name shown on the other device's approval screen and Session list",
				Destination: &label,
			},
		},
		Action: func(c *cli.Context) error {
			return runLoginPair(c, statePath, code, label, c.String("output"))
		},
	}
}

// runLoginPair adds the account offered by another device. Without --code it
// prompts for the code and the emoji comparison; with --code the person on the
// other device compares the emoji and approves there.
func runLoginPair(c *cli.Context, statePath, code, label, outputFormat string) error {
	// Read the code from the flag or an interactive prompt.
	interactive := code == ""
	if interactive {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("pass --code, or run in an interactive terminal")
		}
		var err error
		code, err = (&promptui.Prompt{Label: "Code from your other device"}).Run()
		if err != nil {
			return errors.Wrap(err, "read pairing code")
		}
	}
	code = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if len(code) != 8 {
		return errors.New("pairing code must have 8 characters")
	}

	// Mount a temporary pairing Session on the local provider.
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

	// Connect, verify, and wait for both devices to approve.
	connected, err := sess.CompletePairing(ctx, code, false, strings.TrimSpace(label))
	if err != nil {
		return errors.Wrap(err, "connect to other device")
	}
	remotePeerID := connected.GetRemotePeerId()
	if remotePeerID == "" {
		return errors.New("pairing did not connect to the other device")
	}
	if err := verifyLoginPairing(ctx, sess, remotePeerID, interactive, outputFormat); err != nil {
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

	// Report the new Session.
	if outputFormat == "json" || outputFormat == "yaml" {
		return printSessionListEntry(entry, outputFormat)
	}
	os.Stdout.WriteString("Account connected.\n\n")
	ref := entry.GetSessionRef().GetProviderResourceRef()
	writeFields(os.Stdout, [][2]string{
		{"Provider", ref.GetProviderId()},
		{"Account", ref.GetProviderAccountId()},
		{"Session", ref.GetId()},
		{"Session index", strconv.FormatUint(uint64(entry.GetSessionIndex()), 10)},
	})
	return nil
}

// verifyLoginPairing drives the pairing status until both devices confirm.
// Interactive use compares the emoji here; otherwise this side confirms and
// prints the emoji for the person approving on the other device.
func verifyLoginPairing(ctx context.Context, sess *s4wave_session.Session, remotePeerID string, interactive bool, outputFormat string) error {
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
			emoji := sas.GetEmoji()
			if len(emoji) != 6 {
				return errors.New("pairing did not provide six verification emoji")
			}
			if interactive {
				os.Stdout.WriteString("Confirm these emoji match on both devices:\n" + strings.Join(emoji, " ") + "\n")
				if _, err := (&promptui.Prompt{Label: "Do they match", IsConfirm: true}).Run(); err != nil {
					_ = sess.ConfirmSASMatch(ctx, false)
					return errors.New("pairing cancelled")
				}
			} else if err := printPairingEmoji(emoji, outputFormat); err != nil {
				return err
			}
			if err := sess.ConfirmSASMatch(ctx, true); err != nil {
				return errors.Wrap(err, "confirm emoji")
			}
			confirmed = true
			if outputFormat != "json" && outputFormat != "yaml" {
				os.Stdout.WriteString("Waiting for approval on the other device...\n")
			}
		case s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED:
			if !confirmed {
				return errors.New("other device confirmed before local verification")
			}
			return nil
		case s4wave_session.PairingStatus_PairingStatus_PAIRING_REJECTED:
			return errors.New("pairing rejected on the other device")
		case s4wave_session.PairingStatus_PairingStatus_CONFIRMATION_TIMEOUT:
			return errors.New("pairing was not approved in time; request a new code")
		case s4wave_session.PairingStatus_PairingStatus_FAILED,
			s4wave_session.PairingStatus_PairingStatus_SIGNALING_FAILED,
			s4wave_session.PairingStatus_PairingStatus_CONNECTION_TIMEOUT:
			return errors.Errorf("pairing failed: %s", state.GetErrorMessage())
		}
	}
}

// printPairingEmoji writes the emoji the person approving on the other device
// compares. Structured output emits one record before the final Session record.
func printPairingEmoji(emoji []string, outputFormat string) error {
	if outputFormat != "json" && outputFormat != "yaml" {
		os.Stdout.WriteString("Ask the person approving in Spacewave to check these emoji match:\n" + strings.Join(emoji, " ") + "\n")
		return nil
	}
	buf, ms := newMarshalBuf()
	ms.WriteObjectStart()
	var f bool
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("status")
	ms.WriteString("verify")
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("emoji")
	ms.WriteArrayStart()
	var g bool
	for _, e := range emoji {
		ms.WriteMoreIf(&g)
		ms.WriteString(e)
	}
	ms.WriteArrayEnd()
	ms.WriteObjectEnd()
	if err := formatOutput(buf.Bytes(), outputFormat); err != nil {
		return err
	}
	if outputFormat == "yaml" {
		os.Stdout.WriteString("---\n")
	}
	return nil
}
