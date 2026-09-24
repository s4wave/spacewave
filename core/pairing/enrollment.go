package pairing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// Enrollment retains the selected account and receiving capability until the
// pairing engine releases its preparation. Offering identifies the account
// source, independently of which client created the pairing code.
type Enrollment struct {
	// Choice is the immutable account relationship approved by both clients.
	Choice *AccountChoice
	// Offer binds the selected account to the complete account choice.
	Offer *AccountOffer
	// Identity proves possession of both receiving keys.
	Identity *Identity
	// Receiver installs the approved account on the receiving client.
	Receiver *Receiver
	// Account is the mounted destination account on the receiving client.
	Account provider.ProviderAccount
	// Offering indicates that this client authorizes the selected account.
	Offering bool
	// RemoteLabel is the name the other client gave itself. The authorizing
	// client records it for the receiving Session.
	RemoteLabel string
	// Release drops preparation handles without undoing durable enrollment.
	Release func()
}

// prepare exchanges account identities, fixes the selected outcome, and reserves
// the receiving keys before either client can approve account access.
func (e *Engine) prepare(ctx context.Context, active *attempt, stream *stream_packet.Session, remote peer.ID) (*Enrollment, error) {
	// Describe the selected local account; Home has only a temporary transport
	// and offers only its name.
	local := &AccountOffer{}
	if active.offerCurrent {
		var err error
		local, err = e.adapter.OfferPairingAccount(ctx, e.key)
		if err != nil {
			return nil, err
		}
		if objects, ok := e.adapter.(sobject.SharedObjectProvider); ok {
			list, release, err := objects.AccessSharedObjectList(ctx, nil)
			if err != nil {
				return nil, err
			}
			inventory, err := list.WaitValue(ctx, nil)
			release()
			if err != nil {
				return nil, err
			}
			for _, entry := range inventory.GetSharedObjects() {
				if !entry.GetMeta().GetAccountPrivate() && entry.GetMeta().GetBodyType() == "space" {
					local.SpaceCount++
				}
			}
		}
	}
	if active.label != "" {
		local.MachineName = active.label
	}
	if local.MachineName == "" {
		local.MachineName = defaultMachineName()
	}

	// Order writes by connection role so an unbuffered duplex stream cannot deadlock.
	var remoteOffer *AccountOffer
	if active.offering {
		if err := stream.SendMsg(&Frame{Body: &Frame_Account{Account: local}}); err != nil {
			return nil, err
		}
		frame, err := ReceiveFrame(stream)
		if err != nil {
			return nil, err
		}
		remoteOffer = frame.GetAccount()
	} else {
		frame, err := ReceiveFrame(stream)
		if err != nil {
			return nil, err
		}
		remoteOffer = frame.GetAccount()
		if err := stream.SendMsg(&Frame{Body: &Frame_Account{Account: local}}); err != nil {
			return nil, err
		}
	}

	// Both clients retain the same ordering of account identities in their
	// choice. A name-only offer describes a client with no account.
	remoteLabel := cleanLabel(remoteOffer.GetMachineName())
	choice := &AccountChoice{OfferedAccount: accountOffer(local), ReceivingAccount: accountOffer(remoteOffer)}
	if !active.offering {
		choice.OfferedAccount, choice.ReceivingAccount = choice.ReceivingAccount, choice.OfferedAccount
	}
	if err := choice.ValidateAccounts(); err != nil {
		return nil, err
	}
	if err := e.chooseAccount(ctx, active, stream, choice); err != nil {
		return nil, err
	}

	// Bind both identities and the outcome into the selected account's key proofs.
	offer := choice.GetOfferedAccount().CloneVT()
	offering := active.offering
	if choice.GetOutcome() == AccountOutcome_AccountOutcome_SIGN_IN_RECEIVING || choice.GetOutcome() == AccountOutcome_AccountOutcome_MERGE_INTO_RECEIVING {
		offer = choice.GetReceivingAccount().CloneVT()
		offering = !offering
	}
	data, err := choice.MarshalVT()
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	offer.SelectionContext = hex.EncodeToString(digest[:])
	if offering {
		frame, err := ReceiveFrame(stream)
		if err != nil {
			return nil, err
		}
		identity := frame.GetIdentity()
		if err := ValidateIdentity(offer, identity, e.peerID, remote); err != nil {
			return nil, err
		}
		return &Enrollment{Choice: choice, Offer: offer, Identity: identity, Offering: true, RemoteLabel: remoteLabel, Release: func() {}}, nil
	}

	// The configured provider owns receiving keys, storage, and durable attachment.
	p, releaseProvider, err := provider.ExLookupProvider(ctx, e.b, SessionProviderID(offer), false, nil)
	if err != nil {
		return nil, err
	}
	if p == nil {
		releaseProvider.Release()
		return nil, errors.New("the selected account provider is not configured")
	}
	account, releaseAccount, err := p.AccessProviderAccount(ctx, offer.GetAccountId(), nil)
	if err != nil {
		releaseProvider.Release()
		return nil, err
	}
	release := func() { releaseAccount(); releaseProvider.Release() }
	adapter, ok := account.(AccountAdapter)
	if !ok {
		release()
		return nil, errors.New("the selected account provider does not support pairing")
	}
	receiver, err := adapter.PreparePairingReceiver(ctx, offer, remote, e.peerID)
	if err != nil {
		release()
		return nil, err
	}
	releaseAll := func() { receiver.Release(); release() }
	if err := ValidateIdentity(offer, receiver.Identity, remote, e.peerID); err != nil {
		releaseAll()
		return nil, err
	}
	if err := stream.SendMsg(&Frame{Body: &Frame_Identity{Identity: receiver.Identity}}); err != nil {
		releaseAll()
		return nil, err
	}
	return &Enrollment{Choice: choice, Offer: offer, Identity: receiver.Identity, Receiver: receiver, Account: account, RemoteLabel: remoteLabel, Release: releaseAll}, nil
}

// accountOffer returns offer when it describes an account, or nil for a
// name-only offer.
func accountOffer(offer *AccountOffer) *AccountOffer {
	if offer.GetAccountId() == "" {
		return nil
	}
	return offer
}

// maxLabelRunes bounds the label another client sends. The label is shown on
// the approval screen and stored for the Session it enrolls.
const maxLabelRunes = 64

// cleanLabel reduces a label from another client to printable text of at most
// maxLabelRunes runes.
func cleanLabel(label string) string {
	label = strings.TrimSpace(strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, label))
	if runes := []rune(label); len(runes) > maxLabelRunes {
		label = strings.TrimSpace(string(runes[:maxLabelRunes]))
	}
	return label
}

// defaultMachineName names this client when the caller gave no label.
func defaultMachineName() string {
	name, _ := os.Hostname()
	if name == "" || name == "js" {
		return "Browser"
	}
	return name
}

// chooseAccount lets the code-entering client propose one outcome. The other
// client sees that exact proposal before approving and can reject it. A fixed
// proposal cannot change after either client's approval begins.
func (e *Engine) chooseAccount(ctx context.Context, active *attempt, stream *stream_packet.Session, choice *AccountChoice) error {
	// Home and an already-shared account have only one useful sign-in direction.
	choice.Outcome = AccountOutcome_AccountOutcome_SIGN_IN_OFFERED
	if receiving := choice.GetReceivingAccount(); receiving != nil && !SameAccount(choice.GetOfferedAccount(), receiving) {
		choice.Outcome = AccountOutcome_AccountOutcome_UNSPECIFIED
		e.update(active, func(a *attempt) {
			a.snapshot.Choice = choice.CloneVT()
			a.snapshot.Status = StatusSelectingAccount
		})
	}

	// Receive and validate the entire proposal against the authenticated exchange.
	if active.offering {
		frame, err := ReceiveFrame(stream)
		if err != nil {
			return err
		}
		selected := frame.GetChoice()
		choice.Outcome = selected.GetOutcome()
		if !choice.EqualVT(selected) {
			return errors.New("pairing choice does not match the exchanged accounts")
		}
		if err := choice.Validate(); err != nil {
			return err
		}
		e.update(active, func(a *attempt) { a.snapshot.Choice = choice.CloneVT() })
		return nil
	}

	// Wait on the engine's choice submission while retaining the same operation.
	if choice.GetOutcome() == AccountOutcome_AccountOutcome_UNSPECIFIED {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case choice.Outcome = <-active.choose:
		}
	}
	if err := choice.Validate(); err != nil {
		return err
	}
	e.update(active, func(a *attempt) { a.snapshot.Choice = choice.CloneVT() })
	return stream.SendMsg(&Frame{Body: &Frame_Choice{Choice: choice}})
}
