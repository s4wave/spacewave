package sobject

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ControlAgreement is a change of the held config that voters agree to.
type ControlAgreement struct {
	// Hash is the identity of the change as agreed, without its seal.
	Hash []byte
	// Change is the agreed record, with no sealed checkpoint.
	Change *SOConfigChange
	// Voters are the voters that agree to it, sorted.
	Voters []string
	// Weight is their voting weight.
	Weight uint64
	// value is the agreed encoding.
	value []byte
}

// ControlAgreements returns the changes of the held config voters agree to,
// by most voting weight, then by hash.
func (s *SOState) ControlAgreements() ([]*ControlAgreement, error) {
	msgs, err := s.OpenControlMessages()
	if err != nil {
		return nil, err
	}
	return controlAgreements(s.GetConfig(), msgs)
}

// controlAgreements sums the agreements among msgs.
func controlAgreements(cfg *SharedObjectConfig, msgs []ControlMessage) ([]*ControlAgreement, error) {
	// Collect the voters behind each value.
	byHash := make(map[string]*ControlAgreement)
	for _, m := range msgs {
		inner := m.Inner
		if inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_AGREE {
			continue
		}
		a := byHash[string(inner.GetValueHash())]
		if a == nil {
			change := &SOConfigChange{}
			if err := change.UnmarshalVT(inner.GetValue()); err != nil {
				return nil, errors.Wrap(err, "unmarshal agreed control record")
			}
			a = &ControlAgreement{Hash: inner.GetValueHash(), Change: change, value: inner.GetValue()}
			byHash[string(inner.GetValueHash())] = a
		}
		if !slices.Contains(a.Voters, inner.GetPeerId()) {
			a.Voters = append(a.Voters, inner.GetPeerId())
			a.Weight += cfg.VotingWeight(inner.GetPeerId())
		}
	}

	// Order them by weight, then hash.
	out := make([]*ControlAgreement, 0, len(byHash))
	for _, a := range byHash {
		slices.Sort(a.Voters)
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b *ControlAgreement) int {
		if c := cmp.Compare(b.Weight, a.Weight); c != 0 {
			return c
		}
		return bytes.Compare(a.Hash, b.Hash)
	})
	return out, nil
}

// checkpointHead names the held checkpoint.
func (s *SOState) checkpointHead() (*SOCheckpointHead, error) {
	inner, err := s.GetCheckpointInner()
	if err != nil {
		return nil, err
	}
	if inner == nil {
		return nil, errors.New("state has no checkpoint to seal")
	}
	return &SOCheckpointHead{Height: inner.GetHeight(), Hash: s.GetCheckpoint().Hash()}, nil
}

// unsealControlRecord returns the encoding of entry voters agree to: with no
// sealed checkpoint, signatures or commit.
func unsealControlRecord(entry *SOConfigChange) ([]byte, error) {
	clone := entry.CloneVT()
	clone.GetConfig().SealedCheckpoint = nil
	return configChangeSignedBody(clone)
}

// sealControlRecord returns the encoding of the agreed value with the held
// checkpoint sealed, which a group decides.
func (s *SOState) sealControlRecord(agreed []byte) ([]byte, error) {
	// Decode the agreed record.
	entry := &SOConfigChange{}
	if err := entry.UnmarshalVT(agreed); err != nil {
		return nil, err
	}
	if entry.GetConfig() == nil {
		return nil, errors.New("control record has no config")
	}

	// Seal the held checkpoint head.
	head, err := s.checkpointHead()
	if err != nil {
		return nil, err
	}
	entry.Config.SealedCheckpoint = head
	return configChangeSignedBody(entry)
}

// judgeControlRecord judges a proposed control record as voter self: a change
// a group decides, linked to the held config, sealing the held checkpoint,
// that self agreed to. It waits while too few voters are known to agree.
func (s *SOState) judgeControlRecord(sharedObjectID, self string, msgs []ControlMessage, entry *SOConfigChange) controlValidity {
	// Check the record against the held config and checkpoint.
	cfg := s.GetConfig()
	if !isPeerConfigChange(entry.GetChangeType()) || linkConfigChange(sharedObjectID, cfg, entry) != nil {
		return controlInvalid
	}
	if _, err := configChangeResult(entry); err != nil {
		return controlInvalid
	}
	head, err := s.checkpointHead()
	if err != nil || !head.EqualVT(entry.GetConfig().GetSealedCheckpoint()) {
		return controlInvalid
	}

	// Self, and more than two thirds of the voting weight, agree to it.
	value, err := unsealControlRecord(entry)
	if err != nil {
		return controlInvalid
	}
	h := sha256.Sum256(value)
	agreements, err := controlAgreements(cfg, msgs)
	if err != nil {
		return controlInvalid
	}
	for _, a := range agreements {
		if !bytes.Equal(a.Hash, h[:]) {
			continue
		}
		if !slices.Contains(a.Voters, self) {
			return controlInvalid
		}
		if !HasQuorum(a.Weight, cfg.TotalVotingWeight()) {
			return controlUnknown
		}
		return controlValid
	}
	return controlInvalid
}

// agreedControlRecord returns the sealed control record that self and more
// than two thirds of the voting weight agree to, preferring the most weight,
// or nil.
func (s *SOState) agreedControlRecord(sharedObjectID, self string, msgs []ControlMessage) ([]byte, error) {
	agreements, err := controlAgreements(s.GetConfig(), msgs)
	if err != nil {
		return nil, err
	}
	for _, a := range agreements {
		if !slices.Contains(a.Voters, self) || !HasQuorum(a.Weight, s.GetConfig().TotalVotingWeight()) {
			continue
		}
		sealed, err := s.sealControlRecord(a.value)
		if err != nil {
			return nil, err
		}
		entry := &SOConfigChange{}
		if err := entry.UnmarshalVT(sealed); err != nil {
			return nil, err
		}
		if s.judgeControlRecord(sharedObjectID, self, msgs, entry) == controlValid {
			return sealed, nil
		}
	}
	return nil, nil
}

// ErrAwaitingGroup reports that a voter agreed to a change of a config under
// group control, which applies once the group decides it.
var ErrAwaitingGroup = errors.New("the change waits for the group to agree")

// ChangeSOConfig makes one change of state's config to next. Under owner
// control signer, an owner, signs the change and it applies with fn, as in
// ApplyConfigChange. Under group control signer, a voter, agrees to it and
// ChangeSOConfig returns ErrAwaitingGroup; the group applies it once more than
// two thirds of the voting weight agree, which with two or three voters is all
// of them, and hands the key to the members it admits instead of calling fn.
func ChangeSOConfig(
	ctx context.Context,
	host *SOHost,
	state *SOState,
	next *SharedObjectConfig,
	changeType SOConfigChangeType,
	signer crypto.PrivKey,
	revInfo *SORevocationInfo,
	fn func(state *SOState) error,
) error {
	// Under owner control the owner applies the change at once.
	sharedObjectID := host.GetSharedObjectID()
	current := state.GetConfig()
	if !current.IsGroupControl() {
		return applyOwnerConfigChange(ctx, host, state, next, changeType, signer, revInfo, fn)
	}
	if !isPeerConfigChange(changeType) {
		return errors.Errorf("a group does not decide %v", changeType)
	}

	// Build the change as it would apply after the held checkpoint.
	next = next.CloneVT()
	next.SealedCheckpoint = nil
	entry := newSOConfigChange(sharedObjectID, current, next, changeType)
	entry.RevocationInfo = revInfo
	value, err := configChangeSignedBody(entry)
	if err != nil {
		return err
	}

	// Check the change applies once sealed.
	sealed, err := state.sealControlRecord(value)
	if err != nil {
		return err
	}
	sealedEntry := &SOConfigChange{}
	if err := sealedEntry.UnmarshalVT(sealed); err != nil {
		return err
	}
	if _, err := configChangeResult(sealedEntry); err != nil {
		return err
	}

	// Agree to it without the seal.
	if err := agreeControlRecord(ctx, host, current.GetConfigChainHash(), value, signer); err != nil {
		return err
	}
	return ErrAwaitingGroup
}

// ApproveSOConfigChange has signer, a voter, agree to the change with hash
// that another voter agreed to under the held config.
func ApproveSOConfigChange(ctx context.Context, host *SOHost, hash []byte, signer crypto.PrivKey) error {
	// Find the agreed change.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return err
	}
	agreements, err := state.ControlAgreements()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(agreements, func(a *ControlAgreement) bool { return bytes.Equal(a.Hash, hash) })
	if i == -1 {
		return errors.New("no voter agrees to that change of the held config")
	}
	return agreeControlRecord(ctx, host, state.GetConfig().GetConfigChainHash(), agreements[i].value, signer)
}

// agreeControlRecord adds signer's agreement to the change value of the
// config with head.
func agreeControlRecord(ctx context.Context, host *SOHost, head, value []byte, signer crypto.PrivKey) error {
	// Sign the agreement.
	h := sha256.Sum256(value)
	msg, err := BuildSOControlMessage(signer, &SOControlMessageInner{
		SharedObjectId: host.GetSharedObjectID(),
		Kind:           SODecisionKind_SO_DECISION_KIND_CONFIG,
		ConfigHash:     head,
		Type:           SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_AGREE,
		ValueHash:      h[:],
		Value:          value,
	})
	if err != nil {
		return err
	}

	// Add it while the config it changes is held.
	return host.UpdateSOState(ctx, func(state *SOState) error {
		if !bytes.Equal(state.GetConfig().GetConfigChainHash(), head) {
			return ErrConfigChainHeadMismatch
		}
		_, err := state.AddControlMessage(host.GetSharedObjectID(), msg)
		return err
	})
}

// applyOwnerConfigChange signs the change as signer, an owner, and applies it
// with fn. A change to group control seals the held checkpoint, which must
// still be held when it applies.
func applyOwnerConfigChange(
	ctx context.Context,
	host *SOHost,
	state *SOState,
	next *SharedObjectConfig,
	changeType SOConfigChangeType,
	signer crypto.PrivKey,
	revInfo *SORevocationInfo,
	fn func(state *SOState) error,
) error {
	// Seal the held checkpoint when the group takes control.
	if next.IsGroupControl() {
		head, err := state.checkpointHead()
		if err != nil {
			return err
		}
		next = next.CloneVT()
		next.SealedCheckpoint = head
	}

	// Sign and apply the change.
	entry, err := BuildSOConfigChange(host.GetSharedObjectID(), state.GetConfig(), next, changeType, signer, revInfo)
	if err != nil {
		return errors.Wrap(err, "build config change")
	}
	return host.ApplyConfigChange(ctx, entry, func(applied *SOState) error {
		if next.IsGroupControl() {
			head, err := applied.checkpointHead()
			if err != nil {
				return err
			}
			if !head.EqualVT(next.GetSealedCheckpoint()) {
				return errors.New("the checkpoint moved while the group took control")
			}
		}
		if fn == nil {
			return nil
		}
		return fn(applied)
	})
}

// reconcileGroupGrants hands the key of each epoch the voter of privKey reads
// to the members the held config lets read, and takes it from the rest. A
// grant the config no longer authorizes is wrapped again by the voter.
func reconcileGroupGrants(sharedObjectID string, state *SOState, privKey crypto.PrivKey, self string) error {
	// Find the readers.
	cfg := state.GetConfig()
	var readers []string
	for _, p := range cfg.GetParticipants() {
		if CanReadState(p.GetRole()) && !slices.Contains(readers, p.GetPeerId()) {
			readers = append(readers, p.GetPeerId())
		}
	}
	slices.Sort(readers)

	// Reconcile each epoch the voter reads.
	for _, epoch := range state.GetKeyEpochs() {
		// Take the key from members who no longer read.
		epoch.Grants = slices.DeleteFunc(epoch.Grants, func(g *SOGrant) bool {
			return !slices.Contains(readers, g.GetPeerId())
		})
		own := epoch.FindGrant(self)
		if own == nil {
			continue
		}
		inner, err := own.DecryptInnerData(privKey, sharedObjectID)
		if err != nil {
			return errors.Wrapf(err, "key epoch %d", epoch.GetEpoch())
		}

		// Wrap again each grant the config no longer authorizes, and hand
		// the key to each reader without one.
		for _, reader := range readers {
			i := slices.IndexFunc(epoch.Grants, func(g *SOGrant) bool { return g.GetPeerId() == reader })
			if i != -1 && epoch.Grants[i].ValidateSignature(sharedObjectID, cfg) == nil {
				continue
			}
			_, pub, err := peer.ParsePeerIDWithPubKey(reader)
			if err != nil {
				return err
			}
			grant, err := EncryptSOGrant(privKey, pub, sharedObjectID, inner)
			if err != nil {
				return errors.Wrapf(err, "key epoch %d", epoch.GetEpoch())
			}
			if i == -1 {
				epoch.Grants = append(epoch.Grants, grant)
			} else {
				epoch.Grants[i] = grant
			}
		}
	}
	return nil
}

// ControlHost is an optional interface on SharedObject implementations whose
// local peer can change who controls the shared object and agree to changes
// other voters asked the group for.
type ControlHost interface {
	// SetControl changes who controls the shared object as SetSOControl does,
	// returning ErrAwaitingGroup when the group decides the change.
	SetControl(ctx context.Context, control SOControl) error
	// ApproveConfigChange agrees to the change with hash that another voter
	// agreed to under the held config.
	ApproveConfigChange(ctx context.Context, hash []byte) error
}

// SetSOControl changes who controls host's shared object, signed or agreed to
// by signer. Group control gives one vote to the first writing device of each
// entity, and seals the held checkpoint so the group's decisions follow it.
// Owner control takes every vote away. A group's return to owner control
// needs the group to decide it, so it returns ErrAwaitingGroup. It does
// nothing when control already matches.
func SetSOControl(ctx context.Context, host *SOHost, control SOControl, signer crypto.PrivKey) error {
	// Read the current config.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current SO state")
	}
	current := state.GetConfig()
	if current.GetControl() == control {
		return nil
	}

	// Set the control and its votes.
	next := current.CloneVT()
	next.Control = control
	next.SealedCheckpoint = nil
	if control == SOControl_SO_CONTROL_GROUP {
		next.Participants = DefaultVotingWeights(next.GetParticipants())
	} else {
		for _, p := range next.GetParticipants() {
			p.VotingWeight = 0
		}
	}
	return ChangeSOConfig(ctx, host, state, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SET_CONTROL, signer, nil, nil)
}
