package provider_spacewave

import (
	"context"
	"crypto/rand"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/util/scrub"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// OfferPairingAccount reads membership using the offering Session's authority.
func (a *ProviderAccount) OfferPairingAccount(ctx context.Context, key crypto.PrivKey) (*pairing.AccountOffer, error) {
	peerID, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	client := NewSessionClient(a.p.httpCli, a.p.endpoint, a.p.GetSigningEnvPrefix(), key, peerID.String())
	info, err := client.GetAccountInfo(ctx)
	if err != nil {
		return nil, err
	}
	if info.GetAccountId() != a.accountID {
		return nil, errors.New("offering Session belongs to another cloud account")
	}
	members, err := client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	offer := &pairing.AccountOffer{
		ProviderId: a.GetProviderID(), ProviderEndpoint: a.p.endpoint,
		AccountId: a.accountID, OperationId: ulid.NewULID(), DisplayName: info.GetEntityId(),
	}
	for _, member := range members {
		offer.ActiveSessionPeerIds = append(offer.ActiveSessionPeerIds, member.GetPeerId())
	}
	offer.SessionCount = uint32(len(members)) //nolint:gosec // the cloud response's session list bounds this offer count.
	return offer, nil
}

// PreparePairingReceiver reserves a locally encrypted key before approval.
// The offered endpoint must match the configured provider; it is never dialed.
func (a *ProviderAccount) PreparePairingReceiver(ctx context.Context, offer *pairing.AccountOffer, sourcePeer, receivingPeer peer.ID) (*pairing.Receiver, error) {
	if offer.GetAccountId() != a.accountID || offer.GetProviderEndpoint() != a.p.endpoint {
		return nil, errors.New("the offered cloud account uses a different configured provider")
	}
	controller, releaseController, err := session.ExLookupSessionController(ctx, a.p.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer releaseController.Release()
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	storagePeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	storageKey, err := storagePeer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	ref, key, err := a.reservePairingSession(ctx, offer, entries, storageKey)
	if err != nil {
		return nil, err
	}
	identity, err := pairing.BuildIdentity(offer, ref, key, storageKey, sourcePeer, receivingPeer)
	if err != nil {
		return nil, err
	}
	var releaseSession func()
	var releaseMu sync.Mutex
	var released bool
	return &pairing.Receiver{Transport: func(ctx context.Context) (*transport.SessionTransport, error) {
		sess, release, err := a.MountSession(ctx, ref, nil)
		if err != nil {
			return nil, err
		}
		owner, err := sess.(pairing.Session).GetPairingTransport(ctx, pairing.Relay{})
		if err != nil {
			release()
			return nil, err
		}
		releaseMu.Lock()
		if released {
			releaseMu.Unlock()
			release()
			return nil, context.Canceled
		}
		releaseSession = release
		releaseMu.Unlock()
		return owner, nil
	}, Identity: identity, Release: func() {
		releaseMu.Lock()
		released = true
		release := releaseSession
		releaseSession = nil
		releaseMu.Unlock()
		if release != nil {
			release()
		}
	}, Receive: func(ctx context.Context, stream *stream_packet.Session) error {
		frame, err := pairing.ReceiveFrame(stream)
		if err != nil {
			return err
		}
		if !frame.GetComplete() {
			return errors.New("cloud provider did not finish Session enrollment")
		}
		return a.registerPairedSession(ctx, ref, key)
	}}, nil
}

// EnrollPairingReceiver registers the approved receiving Session with the cloud.
func (a *ProviderAccount) EnrollPairingReceiver(ctx context.Context, stream *stream_packet.Session, enrollment *pairing.Enrollment, sourceKey crypto.PrivKey, sourcePeer, receivingPeer peer.ID) error {
	offer, identity := enrollment.Offer, enrollment.Identity
	if err := pairing.ValidateIdentity(offer, identity, sourcePeer, receivingPeer); err != nil {
		return err
	}
	remotePeer, _, err := peer.ParsePeerIDWithPubKey(identity.GetSessionProof().GetResponderPeerId())
	if err != nil {
		return err
	}
	label := enrollment.RemoteLabel
	if label == "" {
		label = "Paired Session"
	}
	if _, err := a.LinkSession(ctx, sourceKey, remotePeer, label); err != nil {
		return err
	}

	// The cloud observes this client's request, so name the linked Session
	// from its own label. The Session list falls back to its peer ID.
	if enrollment.RemoteLabel != "" {
		observed := &api.ObservedSessionMetadata{Label: label, DeviceType: "linked"}
		if err := a.UpsertSessionPresentation(ctx, remotePeer.String(), observed); err != nil {
			a.le.WithError(err).Warn("failed to name paired Session")
		}
	}
	return stream.SendMsg(&pairing.Frame{Body: &pairing.Frame_Complete{Complete: true}})
}

// reservePairingSession atomically persists a resumable reference and encrypted
// credential. Registered keys absent from the offered membership are replaced.
// PIN-protected Sessions are left untouched and a fresh receiver is reserved.
func (a *ProviderAccount) reservePairingSession(ctx context.Context, offer *pairing.AccountOffer, entries []*session.SessionListEntry, storagePriv crypto.PrivKey) (*session.SessionRef, crypto.PrivKey, error) {
	handle, _, release, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, SessionObjectStoreID(a.accountID), a.vol.GetID(), nil)
	if err != nil {
		return nil, nil, err
	}
	defer release.Release()
	store := handle.GetObjectStore()
	storageKey, err := session_lock.DeriveStorageKey(storagePriv)
	if err != nil {
		return nil, nil, err
	}
	var ref *session.SessionRef
	var key crypto.PrivKey
	binding := []byte("pairing/session-ref")
	err = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		candidates := make([]*session.SessionRef, 0, len(entries)+1)
		for _, entry := range entries {
			candidates = append(candidates, entry.GetSessionRef())
		}
		data, found, err := tx.Get(ctx, binding)
		if err != nil {
			return err
		}
		if found {
			pending := &session.SessionRef{}
			if err := pending.UnmarshalVT(data); err != nil {
				return err
			}
			candidates = append(candidates, pending)
		}
		for _, candidate := range candidates {
			provRef := candidate.GetProviderResourceRef()
			if provRef.GetProviderId() != a.GetProviderID() || provRef.GetProviderAccountId() != a.accountID {
				continue
			}
			id := provRef.GetId()
			marker, registered, err := tx.Get(ctx, []byte(id+"/registered"))
			if err != nil {
				return err
			}
			if registered && !slices.Contains(offer.GetActiveSessionPeerIds(), string(marker)) {
				continue
			}
			data, found, err := tx.Get(ctx, session_lock.MakeKey(id, session_lock.SuffixPK))
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			plain, err := session_lock.DecryptAutoUnlock(storageKey, data)
			if err != nil {
				return err
			}
			key, err = keypem.ParsePrivKeyPem(plain)
			scrub.Scrub(plain)
			if err != nil {
				return err
			}
			ref = candidate
			return nil
		}

		key, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return err
		}
		ref = &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{ProviderId: a.GetProviderID(), ProviderAccountId: a.accountID, Id: ulid.NewULID()}}
		plain, err := keypem.MarshalPrivKeyPem(key)
		if err != nil {
			return err
		}
		defer scrub.Scrub(plain)
		encrypted, err := session_lock.EncryptAutoUnlock(storageKey, plain)
		if err != nil {
			return err
		}
		if err := tx.Set(ctx, session_lock.MakeKey(ref.GetProviderResourceRef().GetId(), session_lock.SuffixPK), encrypted); err != nil {
			return err
		}
		data, err = ref.MarshalVT()
		if err != nil {
			return err
		}
		return tx.Set(ctx, binding, data)
	})
	return ref, key, err
}

// registerPairedSession verifies the receiving key through the configured
// provider before making its approved reference visible in the Session list.
func (a *ProviderAccount) registerPairedSession(ctx context.Context, ref *session.SessionRef, key crypto.PrivKey) error {
	peerID, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}
	client := NewSessionClient(a.p.httpCli, a.p.endpoint, a.p.GetSigningEnvPrefix(), key, peerID.String())
	info, err := client.GetAccountInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "verify receiving Session enrollment")
	}
	if info.GetAccountId() != a.accountID {
		return errors.New("receiving Session was enrolled into another cloud account")
	}
	controller, releaseController, err := session.ExLookupSessionController(ctx, a.p.b, "", false, nil)
	if err != nil {
		return err
	}
	defer releaseController.Release()
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.GetSessionRef().EqualVT(ref) {
			return nil
		}
	}
	if err := a.p.seedHandoffSession(ctx, a, ref, key); err != nil {
		return err
	}
	_, releaseSession, err := a.MountSession(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer releaseSession()
	_, err = controller.RegisterSession(ctx, ref, &session.SessionMetadata{DisplayName: info.GetEntityId(), ProviderDisplayName: "Cloud", ProviderId: a.GetProviderID(), ProviderAccountId: a.accountID, CreatedAt: time.Now().UnixMilli()})
	return err
}
