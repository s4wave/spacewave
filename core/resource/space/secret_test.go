package resource_space

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

func TestSpaceResourceCreateSecretCreatesGrantedSecret(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	readerPriv, readerPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	readerPubPEM, err := keypem.MarshalPubKeyPem(readerPub)
	if err != nil {
		t.Fatal(err)
	}
	readerPeerID, err := peer.IDFromPrivateKey(readerPriv)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey:          "secrets/matrix/access-token",
		DisplayName:        "Matrix access token",
		Kind:               s4wave_secret.SecretKindMatrixAccessToken,
		ContentType:        s4wave_secret.MatrixAccessTokenContentType,
		Value:              []byte("matrix-token"),
		ReaderPublicKeyPem: readerPubPEM,
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if resp.GetSecret().GetRef() == nil {
		t.Fatal("expected nested SharedObjectRef")
	}

	secretResource := s4wave_secret.NewSecretResource(tb.Logger, tb.Bus, tb.WorldState, "secrets/matrix/access-token")
	begin, err := secretResource.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: readerPeerID.String(),
		ExpectedKind: s4wave_secret.SecretKindMatrixAccessToken,
	})
	if err != nil {
		t.Fatalf("BeginReadPayload: %v", err)
	}
	signature, err := peer.NewSignature(
		s4wave_secret.ReadPayloadChallengeSignatureContext,
		readerPriv,
		hash.HashType_HashType_SHA256,
		begin.GetChallenge(),
		true,
	)
	if err != nil {
		t.Fatalf("NewSignature: %v", err)
	}
	read, err := secretResource.ReadPayload(ctx, &s4wave_secret.ReadPayloadRequest{
		ChallengeId: begin.GetChallengeId(),
		Signature:   signature,
	})
	if err != nil {
		t.Fatalf("ReadPayload: %v", err)
	}
	if got := string(read.GetPayload().GetValue()); got != "matrix-token" {
		t.Fatalf("payload mismatch: %q", got)
	}
}

func TestSpaceResourceCreateSecretReconcilesExistingObjectKey(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	req := &s4wave_space.CreateSecretRequest{
		ObjectKey:         "secrets/ssh/password",
		DisplayName:       "SSH password",
		Kind:              s4wave_secret.SecretKindSSHPassword,
		ContentType:       s4wave_secret.SSHTextCredentialContentType,
		Value:             []byte("first"),
		ReconcileExisting: true,
		CreationToken:     []byte("0123456789abcdef0123456789abcdef"),
		PayloadIdentity:   []byte("payload-identity-0123456789abcdef"),
		Timestamp:         timestamppb.New(time.Unix(100, 0)),
	}
	first, err := resource.CreateSecret(ctx, req)
	if err != nil {
		t.Fatalf("first CreateSecret: %v", err)
	}
	second, err := resource.CreateSecret(ctx, req)
	if err != nil {
		t.Fatalf("retry CreateSecret: %v", err)
	}
	if first.GetSecret().GetNestedSharedObjectId() != second.GetSecret().GetNestedSharedObjectId() {
		t.Fatal("retry created a second nested secret")
	}

	req.ReconcileExisting = false
	if _, err := resource.CreateSecret(ctx, req); !errors.Is(err, world.ErrObjectExists) {
		t.Fatalf("expected default create collision, got %v", err)
	}
	req.ReconcileExisting = true

	req.Kind = s4wave_secret.SecretKindSSHPrivateKey
	if _, err := resource.CreateSecret(ctx, req); err == nil {
		t.Fatal("expected an existing-kind mismatch")
	}
	req.Kind = s4wave_secret.SecretKindSSHPassword
	req.ContentType = "application/octet-stream"
	if _, err := resource.CreateSecret(ctx, req); err == nil {
		t.Fatal("expected an existing-content-type mismatch")
	}
	req.ContentType = s4wave_secret.SSHTextCredentialContentType
	req.Value = []byte("second")
	if _, err := resource.CreateSecret(ctx, req); err == nil {
		t.Fatal("expected an existing-value mismatch")
	}
	req.Value = []byte("first")
	req.CreationToken = []byte("abcdef0123456789abcdef0123456789")
	if _, err := resource.CreateSecret(ctx, req); err == nil {
		t.Fatal("expected an existing-creation-token mismatch")
	}

	_, unauthorizedPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	unauthorizedPeerID, err := peer.IDFromPublicKey(unauthorizedPub)
	if err != nil {
		t.Fatal(err)
	}
	unauthorizedPEM, err := keypem.MarshalPubKeyPem(unauthorizedPub)
	if err != nil {
		t.Fatal(err)
	}
	req.CreationToken = []byte("0123456789abcdef0123456789abcdef")
	req.Value = []byte("wrong-value")
	req.ReaderPublicKeyPem = unauthorizedPEM
	if _, err := resource.CreateSecret(ctx, req); err == nil {
		t.Fatal("expected wrong-value grant request to fail")
	}
	if _, err := s4wave_secret.ReadSecretPayloadForPeer(
		ctx,
		tb.Bus,
		first.GetSecret(),
		s4wave_secret.SecretKindSSHPassword,
		unauthorizedPeerID.String(),
	); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("wrong-value request granted unauthorized reader: %v", err)
	}
}

func TestSpaceResourceCreateSecretRejectsMatchingBodyWithWrongObjectType(t *testing.T) {
	ctx := t.Context()
	_, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	const objectKey = "secrets/ssh/wrong-type"
	tx, err := resource.space.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := world.CreateWorldObject(ctx, tx, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&s4wave_secret.Secret{
			DisplayName:   "SSH password",
			Kind:          s4wave_secret.SecretKindSSHPassword,
			CreationToken: []byte("0123456789abcdef0123456789abcdef"),
		}, true)
		return nil
	}); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tx, objectKey, "wrong/type"); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey:         objectKey,
		DisplayName:       "SSH password",
		Kind:              s4wave_secret.SecretKindSSHPassword,
		ContentType:       s4wave_secret.SSHTextCredentialContentType,
		Value:             []byte("password"),
		ReconcileExisting: true,
		CreationToken:     []byte("0123456789abcdef0123456789abcdef"),
		PayloadIdentity:   []byte("payload-identity-0123456789abcdef"),
		Timestamp:         timestamppb.New(time.Unix(100, 0)),
	})
	if err == nil {
		t.Fatal("matching body with the wrong ObjectType was accepted")
	}
}

func TestSpaceResourceDeleteSecretDeletesNestedBeforeParentAndRetries(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()
	token := []byte("0123456789abcdef0123456789abcdef")
	created, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey: "secrets/delete", DisplayName: "Delete", Kind: s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType, Value: []byte("value"),
		ReconcileExisting: true, CreationToken: token, PayloadIdentity: []byte("payload-identity-0123456789abcdef"), Timestamp: timestamppb.New(time.Unix(100, 0)),
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := resource.DeleteSecret(ctx, &s4wave_space.DeleteSecretRequest{ObjectKey: "secrets/delete", CreationToken: token, NestedSharedObjectId: created.GetSecret().GetNestedSharedObjectId()})
	if err != nil || !resp.GetDeleted() {
		t.Fatalf("DeleteSecret: %v, %+v", err, resp)
	}
	if _, found, err := tb.WorldState.GetObject(ctx, "secrets/delete"); err != nil || found {
		t.Fatalf("parent remains: found=%v err=%v", found, err)
	}
	soProvider, releaseProvider := accessSecretTestProvider(ctx, t, resource)
	defer releaseProvider()
	found, matches, err := secretSharedObjectMatchesToken(ctx, soProvider, created.GetSecret().GetNestedSharedObjectId(), token)
	if err != nil {
		t.Fatal(err)
	}
	if found || matches {
		t.Fatal("nested payload remains after parent cleanup")
	}
	resp, err = resource.DeleteSecret(ctx, &s4wave_space.DeleteSecretRequest{ObjectKey: "secrets/delete", CreationToken: token, NestedSharedObjectId: created.GetSecret().GetNestedSharedObjectId()})
	if err != nil || resp.GetDeleted() {
		t.Fatalf("retry DeleteSecret: %v, %+v", err, resp)
	}
	_ = created
}

func TestSpaceResourceDeleteSecretResumesAfterNestedDeletionCrash(t *testing.T) {
	ctx := t.Context()
	_, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()
	token := []byte("0123456789abcdef0123456789abcdef")
	created, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey: "secrets/delete-crash", DisplayName: "Delete crash", Kind: s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType, Value: []byte("value"),
		ReconcileExisting: true, CreationToken: token, PayloadIdentity: []byte("payload-identity-0123456789abcdef"), Timestamp: timestamppb.New(time.Unix(100, 0)),
	})
	if err != nil {
		t.Fatal(err)
	}
	soProvider, releaseProvider := accessSecretTestProvider(ctx, t, resource)
	defer releaseProvider()
	if err := soProvider.DeleteSharedObject(ctx, created.GetSecret().GetNestedSharedObjectId()); err != nil {
		t.Fatal(err)
	}

	resp, err := resource.DeleteSecret(ctx, &s4wave_space.DeleteSecretRequest{
		ObjectKey: "secrets/delete-crash", CreationToken: token,
		NestedSharedObjectId: created.GetSecret().GetNestedSharedObjectId(),
	})
	if err != nil || !resp.GetDeleted() {
		t.Fatalf("DeleteSecret after crash: resp=%+v err=%v", resp, err)
	}
}

func TestSpaceResourceDeleteSecretRejectsReplacementBeforeCommit(t *testing.T) {
	ctx := t.Context()
	_, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	const objectKey = "secrets/delete-replaced"
	oldToken := []byte("old-token-0123456789abcdef01234567")
	oldSecret, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey: objectKey, DisplayName: "Old", Kind: s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType, Value: []byte("old-value"),
		ReconcileExisting: true, CreationToken: oldToken, PayloadIdentity: []byte("old-payload-identity-0123456789ab"), Timestamp: timestamppb.New(time.Unix(100, 0)),
	})
	if err != nil {
		t.Fatal(err)
	}

	soProvider, releaseProvider := accessSecretTestProvider(ctx, t, resource)
	defer releaseProvider()
	replacement := oldSecret.GetSecret().CloneVT()
	replacement.DisplayName = "Replacement"
	replacement.PayloadIdentity = []byte("new-payload-identity-0123456789ab")
	replacement.UpdatedAt = timestamppb.New(time.Unix(200, 0))

	body := resource.space.(*secretSpaceBody)
	baseEngine := body.engine
	body.engine = &deleteBarrierEngine{Engine: baseEngine, beforeWrite: func(ctx context.Context) error {
		tx, err := baseEngine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		if _, err := tx.DeleteObject(ctx, objectKey); err != nil {
			return err
		}
		createdState, _, err := world.CreateWorldObject(ctx, tx, objectKey, func(bcs *block.Cursor) error {
			bcs.SetBlock(replacement, true)
			return nil
		})
		world.ReleaseObjectState(createdState)
		if err != nil {
			return err
		}
		if err := world_types.SetObjectType(ctx, tx, objectKey, s4wave_secret.SecretTypeID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}}

	_, err = resource.DeleteSecret(ctx, &s4wave_space.DeleteSecretRequest{
		ObjectKey: objectKey, CreationToken: oldToken,
		NestedSharedObjectId: oldSecret.GetSecret().GetNestedSharedObjectId(),
	})
	if err == nil {
		t.Fatal("stale delete committed over replacement")
	}

	readTx, err := baseEngine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	persisted, objRef, err := world.LookupObject[*s4wave_secret.Secret](ctx, readTx, objectKey, s4wave_secret.NewSecretBlock)
	world.ReleaseObjectState(objRef)
	if err == nil {
		err = world_types.CheckObjectType(ctx, readTx, objectKey, s4wave_secret.SecretTypeID)
	}
	readTx.Discard()
	if err != nil {
		t.Fatalf("replacement parent missing: %v", err)
	}
	if !persisted.EqualVT(replacement) {
		t.Fatalf("replacement parent changed: got %+v want %+v", persisted, replacement)
	}
	found, matches, err := secretSharedObjectMatchesToken(ctx, soProvider, replacement.GetNestedSharedObjectId(), oldToken)
	if err != nil || !found || !matches {
		t.Fatalf("replacement nested object missing: found=%v matches=%v err=%v", found, matches, err)
	}
}

func TestSpaceResourceCreateSecretRejectsUnauthorizedSessionWithoutMutation(t *testing.T) {
	for _, withReader := range []bool{false, true} {
		name := "without-reader"
		if withReader {
			name = "with-reader"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			_, resource, release := setupSecretSpaceResourceTest(ctx, t)
			defer release()

			providerSnapshot := func() *sobject.SharedObjectList {
				soProvider, releaseProvider := accessSecretTestProvider(ctx, t, resource)
				defer releaseProvider()
				ctr, releaseList, err := soProvider.AccessSharedObjectList(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer releaseList()
				list, err := ctr.WaitValue(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				return list.CloneVT()
			}
			worldKeys := func() []string {
				tx, err := resource.space.GetWorldEngine().NewTransaction(ctx, false)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Discard()
				iter := tx.IterateObjects(ctx, "", false)
				defer iter.Close()
				var keys []string
				for iter.Next() {
					keys = append(keys, iter.Key())
				}
				if err := iter.Err(); err != nil {
					t.Fatal(err)
				}
				return keys
			}

			beforeProvider := providerSnapshot()
			beforeWorld := worldKeys()
			beforeState, err := resource.space.GetSharedObject().GetSharedObjectState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			beforeRoot, err := beforeState.GetRootState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			beforeRoot = beforeRoot.CloneVT()
			beforeParticipant, err := beforeState.GetParticipantConfig(ctx)
			if err != nil {
				t.Fatal(err)
			}
			beforeParticipant = beforeParticipant.CloneVT()

			req := &s4wave_space.CreateSecretRequest{
				ObjectKey:   "secrets/unauthorized-" + name,
				DisplayName: "Unauthorized",
				Kind:        s4wave_secret.SecretKindSSHPassword,
				ContentType: s4wave_secret.SSHTextCredentialContentType,
				Value:       []byte("value"),
			}
			if withReader {
				_, readerPub, err := crypto.GenerateEd25519Key(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				req.ReaderPublicKeyPem, err = keypem.MarshalPubKeyPem(readerPub)
				if err != nil {
					t.Fatal(err)
				}
			}

			resource.sessionPeerID = "unauthorized-caller"
			if _, err := resource.CreateSecret(ctx, req); err == nil {
				t.Fatal("create succeeded without Space writer authority")
			}

			afterProvider := providerSnapshot()
			if !beforeProvider.EqualVT(afterProvider) {
				t.Fatalf("unauthorized request mutated provider SharedObjects: before=%+v after=%+v", beforeProvider, afterProvider)
			}
			afterWorld := worldKeys()
			if !slices.Equal(beforeWorld, afterWorld) {
				t.Fatalf("unauthorized request mutated Space objects: before=%v after=%v", beforeWorld, afterWorld)
			}
			afterState, err := resource.space.GetSharedObject().GetSharedObjectState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			afterRoot, err := afterState.GetRootState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !beforeRoot.EqualVT(afterRoot) {
				t.Fatal("unauthorized request mutated the Space SharedObject root")
			}
			afterParticipant, err := afterState.GetParticipantConfig(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !beforeParticipant.EqualVT(afterParticipant) {
				t.Fatal("unauthorized request mutated the Space participant")
			}
		})
	}
}

func TestSpaceResourceReadSecretPayloadUsesMountedSessionGrant(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	readerPub, err := tb.Volume.GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	readerPubPEM, err := keypem.MarshalPubKeyPem(readerPub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey:          "secrets/matrix/session-token",
		DisplayName:        "Matrix access token",
		Kind:               s4wave_secret.SecretKindMatrixAccessToken,
		ContentType:        s4wave_secret.MatrixAccessTokenContentType,
		Value:              []byte("session-matrix-token"),
		ReaderPublicKeyPem: readerPubPEM,
	}); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	read, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/matrix/session-token",
		ExpectedKind: s4wave_secret.SecretKindMatrixAccessToken,
	})
	if err != nil {
		t.Fatalf("ReadSecretPayload: %v", err)
	}
	if read.GetSecret().GetKind() != s4wave_secret.SecretKindMatrixAccessToken {
		t.Fatalf("secret kind: got %q", read.GetSecret().GetKind())
	}
	if got := string(read.GetPayload().GetValue()); got != "session-matrix-token" {
		t.Fatalf("payload mismatch: %q", got)
	}
	if _, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/matrix/session-token",
		ExpectedKind: "other",
	}); !errors.Is(err, s4wave_secret.ErrSecretKindMismatch) {
		t.Fatalf("expected kind mismatch, got %v", err)
	}

	otherPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPeerID, err := peer.IDFromPrivateKey(otherPriv)
	if err != nil {
		t.Fatal(err)
	}
	resource.sessionPeerID = otherPeerID.String()
	if _, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/matrix/session-token",
		ExpectedKind: s4wave_secret.SecretKindMatrixAccessToken,
	}); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected payload access denied, got %v", err)
	}
}

func TestSpaceResourceCreateSecretAdoptsNestedPayloadAfterCrash(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	token := []byte("0123456789abcdef0123456789abcdef")
	objectKey := "secrets/crash-adoption"
	nestedID := secretNestedSharedObjectID(token, objectKey)
	soProvider, releaseProvider := accessSecretTestProvider(ctx, t, resource)
	defer releaseProvider()
	meta := s4wave_secret.NewSharedObjectMeta()
	meta.BodyMeta = append([]byte(nil), token...)
	nestedRef, err := soProvider.CreateSharedObject(ctx, nestedID, meta, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Unix(100, 0)
	if err := s4wave_secret.StoreSecretPayload(ctx, tb.Bus, nestedRef, &s4wave_secret.SecretPayload{
		Value: []byte("password"), ContentType: s4wave_secret.SSHTextCredentialContentType,
		Version: 1, UpdatedAt: timestamppb.New(ts), PayloadIdentity: []byte("payload-identity-0123456789abcdef"),
	}); err != nil {
		t.Fatal(err)
	}

	created, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey: objectKey, DisplayName: "Crash adoption", Kind: s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType, Value: []byte("password"),
		ReconcileExisting: true, CreationToken: token, PayloadIdentity: []byte("payload-identity-0123456789abcdef"), Timestamp: timestamppb.New(ts),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if created.GetSecret().GetNestedSharedObjectId() != nestedID {
		t.Fatalf("nested id = %q, want %q", created.GetSecret().GetNestedSharedObjectId(), nestedID)
	}
}

func TestSpaceResourceCreateSecretConcurrentRetryRereadsWinner(t *testing.T) {
	ctx := t.Context()
	_, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	req := &s4wave_space.CreateSecretRequest{
		ObjectKey: "secrets/concurrent", DisplayName: "Concurrent", Kind: s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType, Value: []byte("password"),
		ReconcileExisting: true, CreationToken: []byte("0123456789abcdef0123456789abcdef"),
		PayloadIdentity: []byte("payload-identity-0123456789abcdef"), Timestamp: timestamppb.New(time.Unix(100, 0)),
	}
	start := make(chan struct{})
	responses := make([]*s4wave_space.CreateSecretResponse, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Go(func() {
			<-start
			responses[i], errs[i] = resource.CreateSecret(ctx, req.CloneVT())
		})
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("CreateSecret[%d]: %v", i, err)
		}
	}
	if responses[0].GetSecret().GetNestedSharedObjectId() != responses[1].GetSecret().GetNestedSharedObjectId() {
		t.Fatal("concurrent retries did not converge on one nested payload")
	}
}

func TestSpaceResourceCreateSecretConcurrentDifferentPayloadChoosesOneWinner(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	base := &s4wave_space.CreateSecretRequest{
		ObjectKey: "secrets/concurrent-different", DisplayName: "Concurrent different", Kind: s4wave_secret.SecretKindSSHPassword,
		ContentType:       s4wave_secret.SSHTextCredentialContentType,
		ReconcileExisting: true, CreationToken: []byte("0123456789abcdef0123456789abcdef"),
		Timestamp: timestamppb.New(time.Unix(100, 0)),
	}
	requests := []*s4wave_space.CreateSecretRequest{base.CloneVT(), base.CloneVT()}
	requests[0].Value = []byte("alpha-password")
	requests[0].PayloadIdentity = []byte("alpha-payload-identity-0123456789")
	requests[1].Value = []byte("bravo-password")
	requests[1].PayloadIdentity = []byte("bravo-payload-identity-0123456789")

	start := make(chan struct{})
	responses := make([]*s4wave_space.CreateSecretResponse, len(requests))
	errs := make([]error, len(requests))
	var wg sync.WaitGroup
	for i := range requests {
		wg.Go(func() {
			<-start
			responses[i], errs[i] = resource.CreateSecret(ctx, requests[i])
		})
	}
	close(start)
	wg.Wait()

	winner := -1
	for i, err := range errs {
		if err == nil {
			if winner != -1 {
				t.Fatalf("both different payload requests succeeded: responses=%+v", responses)
			}
			winner = i
		}
	}
	if winner == -1 {
		t.Fatalf("neither payload request succeeded: %v", errs)
	}
	persisted, err := s4wave_secret.ReadSecretPayload(ctx, tb.Bus, responses[winner].GetSecret())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(persisted.GetValue(), requests[winner].GetValue()) {
		t.Fatalf("persisted payload = %q, winner requested %q", persisted.GetValue(), requests[winner].GetValue())
	}
	if !bytes.Equal(persisted.GetPayloadIdentity(), requests[winner].GetPayloadIdentity()) ||
		!bytes.Equal(responses[winner].GetSecret().GetPayloadIdentity(), requests[winner].GetPayloadIdentity()) {
		t.Fatal("parent and nested payload did not bind the winning content identity")
	}
}

func TestSpaceResourceCreateSecretValidatesImmutableTimestamps(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	req := &s4wave_space.CreateSecretRequest{
		ObjectKey: "secrets/immutable", DisplayName: "Immutable", Kind: s4wave_secret.SecretKindSSHPassword,
		ContentType: s4wave_secret.SSHTextCredentialContentType, Value: []byte("password"),
		ReconcileExisting: true, CreationToken: []byte("0123456789abcdef0123456789abcdef"),
		PayloadIdentity: []byte("payload-identity-0123456789abcdef"), Timestamp: timestamppb.New(time.Unix(100, 0)),
	}
	created, err := resource.CreateSecret(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	wrongTimestamp := req.CloneVT()
	wrongTimestamp.Timestamp = timestamppb.New(time.Unix(101, 0))
	if _, err := resource.CreateSecret(ctx, wrongTimestamp); err == nil {
		t.Fatal("parent timestamp drift was accepted")
	}
	payload, err := s4wave_secret.ReadSecretPayload(ctx, tb.Bus, created.GetSecret())
	if err != nil {
		t.Fatal(err)
	}
	payload.UpdatedAt = timestamppb.New(time.Unix(102, 0))
	if err := s4wave_secret.StoreSecretPayload(ctx, tb.Bus, created.GetSecret().GetRef(), payload); err != nil {
		t.Fatal(err)
	}
	if _, err := resource.CreateSecret(ctx, req); err == nil {
		t.Fatal("payload timestamp drift was accepted")
	}
}

type deleteBarrierEngine struct {
	world.Engine
	beforeWrite func(context.Context) error
}

func (e *deleteBarrierEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if write && e.beforeWrite != nil {
		beforeWrite := e.beforeWrite
		e.beforeWrite = nil
		if err := beforeWrite(ctx); err != nil {
			return nil, err
		}
	}
	return e.Engine.NewTransaction(ctx, write)
}

type secretSpaceBody struct {
	ref    *sobject.SharedObjectRef
	engine world.Engine
	so     sobject.SharedObject
}

func (b *secretSpaceBody) GetWorldEngine() world.Engine {
	return b.engine
}

func (b *secretSpaceBody) GetWorldEngineID() string {
	return "test-secret-space-engine"
}

func (b *secretSpaceBody) GetWorldEngineBucketID() string {
	return "test-secret-space-bucket"
}

func (b *secretSpaceBody) GetSharedObjectRef() *sobject.SharedObjectRef {
	return b.ref
}

func (b *secretSpaceBody) GetSharedObject() sobject.SharedObject {
	return b.so
}

func accessSecretTestProvider(
	ctx context.Context,
	t *testing.T,
	resource *SpaceResource,
) (sobject.SharedObjectProvider, func()) {
	t.Helper()
	ref := resource.space.GetSharedObjectRef().GetProviderResourceRef()
	account, release, err := provider.ExAccessProviderAccount(
		ctx, resource.b, ref.GetProviderId(), ref.GetProviderAccountId(), false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, account)
	if err != nil {
		release.Release()
		t.Fatal(err)
	}
	return soProvider, release.Release
}

func setupSecretSpaceResourceTest(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, *SpaceResource, func()) {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	accountID := "test-account-" + sobject.NewSOOperationLocalID()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	spaceMeta, err := space.NewSharedObjectMeta("Secret Test Space")
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	spaceRef, err := soProvider.CreateSharedObject(ctx, "space-"+sobject.NewSOOperationLocalID(), spaceMeta, "", "")
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	mounted, mountedRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, spaceRef, false, nil)
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	resource := &SpaceResource{
		le:            tb.Logger,
		b:             tb.Bus,
		space:         &secretSpaceBody{ref: spaceRef, engine: tb.BusEngine, so: mounted},
		sessionPeerID: mounted.GetPeerID().String(),
	}
	return tb, resource, func() {
		mountedRef.Release()
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
}
