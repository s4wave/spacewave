package sobject

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

func TestReencryptSOState(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 3)
	actor, reader, excluded := peers[0], peers[1], peers[2]
	actorPriv, err := actor.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	readerPriv, err := reader.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	le := logrus.New().WithField("test", t.Name())
	sourceID := "source-reencrypt-test"
	destinationID := "destination-reencrypt-test"
	sourceParticipants := []*SOParticipantConfig{
		{PeerId: actor.GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: reader.GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_READER},
	}
	sourceTransformConf, sourceGrants, _, err := RotateTransformKey(
		actorPriv,
		sourceID,
		sourceParticipants,
		4,
		7,
	)
	if err != nil {
		t.Fatalf("build source transform: %v", err)
	}
	sourceTransform, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le}, sfs, sourceTransformConf,
	)
	if err != nil {
		t.Fatalf("build source transformer: %v", err)
	}
	sourceInner := &SORootInner{Seqno: 7, StateData: []byte("source payload")}
	sourceInnerData := mustMarshalVT(t, sourceInner)
	sourceInnerEnc, err := sourceTransform.EncodeBlock(sourceInnerData)
	if err != nil {
		t.Fatalf("encrypt source root: %v", err)
	}
	sourceRoot := &SORoot{
		Inner:      sourceInnerEnc,
		InnerSeqno: 7,
		AccountNonces: []*SOAccountNonce{{
			PeerId: actor.GetPeerID().String(),
			Nonce:  9,
		}},
	}
	if err := sourceRoot.SignInnerData(actorPriv, sourceID, 7, hash.RecommendedHashType); err != nil {
		t.Fatalf("sign source root: %v", err)
	}
	sourceState := &SOState{
		Config: &SharedObjectConfig{
			Participants:     sourceParticipants,
			ConfigChainHash:  []byte("source config history"),
			ConfigChainSeqno: 12,
		},
		Root:       sourceRoot,
		RootGrants: sourceGrants,
		Ops:        []*SOOperation{{Inner: []byte("source pending op")}},
		OpRejections: []*SOPeerOpRejections{{
			PeerId:     actor.GetPeerID().String(),
			Rejections: []*SOOperationRejection{{Inner: []byte("source rejection")}},
		}},
		QueuedAccountNonces: []*SOAccountNonce{{
			PeerId: actor.GetPeerID().String(),
			Nonce:  10,
		}},
		Invites: []*SOInvite{{InviteId: "source invite"}},
	}
	sourceBefore := mustMarshalVT(t, sourceState)

	destinationParticipants := []*SOParticipantConfig{
		{PeerId: actor.GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: reader.GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_READER},
	}
	first, err := ReencryptSOState(
		ctx,
		le,
		sfs,
		sourceID,
		sourceState,
		actorPriv,
		destinationID,
		actorPriv,
		destinationParticipants,
	)
	if err != nil {
		t.Fatalf("reencrypt source state: %v", err)
	}
	second, err := ReencryptSOState(
		ctx,
		le,
		sfs,
		sourceID,
		sourceState,
		actorPriv,
		destinationID,
		actorPriv,
		destinationParticipants,
	)
	if err != nil {
		t.Fatalf("reencrypt source state second run: %v", err)
	}

	if got := mustMarshalVT(t, sourceState); !bytes.Equal(got, sourceBefore) {
		t.Fatal("re-encryption mutated source state")
	}
	if first.GetRoot().GetInnerSeqno() != 1 || len(first.GetRoot().GetAccountNonces()) != 0 {
		t.Fatalf("destination root history was not reset: seqno=%d nonces=%d", first.GetRoot().GetInnerSeqno(), len(first.GetRoot().GetAccountNonces()))
	}
	if first.GetConfig().GetConfigChainSeqno() != 0 || len(first.GetConfig().GetConfigChainHash()) != 0 {
		t.Fatal("destination config history was copied")
	}
	if len(first.GetOps()) != 0 || len(first.GetOpRejections()) != 0 || len(first.GetQueuedAccountNonces()) != 0 || len(first.GetInvites()) != 0 {
		t.Fatal("destination operational history was copied")
	}
	if len(first.GetRoot().GetValidatorSignatures()) != 1 {
		t.Fatalf("destination root should have one fresh validator signature, got %d", len(first.GetRoot().GetValidatorSignatures()))
	}
	sourceSignatureData := mustMarshalVT(t, sourceRoot.GetValidatorSignatures()[0])
	destinationSignatureData := mustMarshalVT(t, first.GetRoot().GetValidatorSignatures()[0])
	if bytes.Equal(sourceSignatureData, destinationSignatureData) {
		t.Fatal("destination reused a source root signature")
	}
	if bytes.Equal(first.GetRoot().GetInner(), second.GetRoot().GetInner()) {
		t.Fatal("two runs reused root ciphertext")
	}

	firstActor := NewSOStateParticipantHandle(le, sfs, destinationID, first, actorPriv, actor.GetPeerID())
	firstActorInner, err := firstActor.GetRootInner(ctx)
	if err != nil {
		t.Fatalf("decode destination root as actor: %v", err)
	}
	if !bytes.Equal(firstActorInner.GetStateData(), sourceInner.GetStateData()) {
		t.Fatalf("payload mismatch: got %q want %q", firstActorInner.GetStateData(), sourceInner.GetStateData())
	}
	firstReader := NewSOStateParticipantHandle(le, sfs, destinationID, first, readerPriv, reader.GetPeerID())
	if _, err := firstReader.GetRootInner(ctx); err != nil {
		t.Fatalf("selected reader could not decode destination root: %v", err)
	}
	if len(first.GetRootGrants()) != 2 {
		t.Fatalf("expected grants for actor and selected reader, got %d", len(first.GetRootGrants()))
	}
	if _, err := first.GetRootGrants()[0].DecryptInnerData(actorPriv, destinationID); err != nil {
		t.Fatalf("actor grant was not readable: %v", err)
	}
	if _, err := first.GetRootGrants()[1].DecryptInnerData(readerPriv, destinationID); err != nil {
		t.Fatalf("reader grant was not readable: %v", err)
	}
	for _, grant := range first.GetRootGrants() {
		if grant.GetPeerId() == excluded.GetPeerID().String() {
			t.Fatal("destination included a non-selected participant grant")
		}
	}
	firstGrantInner, err := first.GetRootGrants()[0].DecryptInnerData(actorPriv, destinationID)
	if err != nil {
		t.Fatal(err)
	}
	secondGrantInner, err := second.GetRootGrants()[0].DecryptInnerData(actorPriv, destinationID)
	if err != nil {
		t.Fatal(err)
	}
	firstTransformData := mustMarshalVT(t, firstGrantInner.GetTransformConf())
	secondTransformData := mustMarshalVT(t, secondGrantInner.GetTransformConf())
	if bytes.Equal(firstTransformData, secondTransformData) {
		t.Fatal("two runs reused transform key material")
	}
	if err := first.Validate(destinationID); err != nil {
		t.Fatalf("destination state validation: %v", err)
	}
}

func TestReencryptSOStateRejectsUnreadableSourceAndActor(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 2)
	sourceActor, destinationActor := peers[0], peers[1]
	sourcePriv, err := sourceActor.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	destinationPriv, err := destinationActor.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	le := logrus.New().WithField("test", t.Name())
	sourceID := "unreadable-reencrypt-source"
	sourceParticipants := []*SOParticipantConfig{{
		PeerId: sourceActor.GetPeerID().String(),
		Role:   SOParticipantRole_SOParticipantRole_OWNER,
	}}
	sourceTransformConf, sourceGrants, _, err := RotateTransformKey(sourcePriv, sourceID, sourceParticipants, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	sourceTransform, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, sourceTransformConf)
	if err != nil {
		t.Fatal(err)
	}
	sourceInnerData := mustMarshalVT(t, &SORootInner{Seqno: 1, StateData: []byte("payload")})
	sourceInnerEnc, err := sourceTransform.EncodeBlock(sourceInnerData)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := &SORoot{Inner: sourceInnerEnc, InnerSeqno: 1}
	if err := sourceRoot.SignInnerData(sourcePriv, sourceID, 1, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
	unreadable := &SOState{
		Config:     &SharedObjectConfig{Participants: sourceParticipants},
		Root:       sourceRoot,
		RootGrants: nil,
	}
	destinationParticipants := []*SOParticipantConfig{{
		PeerId: destinationActor.GetPeerID().String(),
		Role:   SOParticipantRole_SOParticipantRole_OWNER,
	}}
	if _, err := ReencryptSOState(ctx, le, sfs, sourceID, unreadable, sourcePriv, "unreadable-destination", destinationPriv, destinationParticipants); err == nil {
		t.Fatal("unreadable source was accepted")
	}

	readable := unreadable.CloneVT()
	readable.RootGrants = sourceGrants

	blankRoot := readable.CloneVT()
	blankRoot.Root = &SORoot{}
	if _, err := ReencryptSOState(ctx, le, sfs, sourceID, blankRoot, sourcePriv, "blank-root-destination", destinationPriv, destinationParticipants); err == nil {
		t.Fatal("blank source root was accepted")
	}

	tamperedRoot := readable.CloneVT()
	tamperedRoot.Root.Inner = append([]byte(nil), tamperedRoot.Root.GetInner()...)
	tamperedRoot.Root.Inner[0] ^= 0xff
	if _, err := ReencryptSOState(ctx, le, sfs, sourceID, tamperedRoot, sourcePriv, "tampered-root-destination", destinationPriv, destinationParticipants); err == nil {
		t.Fatal("tampered source root was accepted")
	}

	unsignedRoot := readable.CloneVT()
	unsignedRoot.Root.ValidatorSignatures = nil
	if _, err := ReencryptSOState(ctx, le, sfs, sourceID, unsignedRoot, sourcePriv, "unsigned-root-destination", destinationPriv, destinationParticipants); err == nil {
		t.Fatal("unsigned source root was accepted")
	}

	forgedGrant := readable.CloneVT()
	forgedGrant.RootGrants[0].InnerData = append([]byte(nil), forgedGrant.RootGrants[0].GetInnerData()...)
	forgedGrant.RootGrants[0].InnerData[0] ^= 0xff
	if _, err := ReencryptSOState(ctx, le, sfs, sourceID, forgedGrant, sourcePriv, "forged-grant-destination", destinationPriv, destinationParticipants); err == nil {
		t.Fatal("forged source grant was accepted")
	}
	notReadableDestination := []*SOParticipantConfig{{
		PeerId: sourceActor.GetPeerID().String(),
		Role:   SOParticipantRole_SOParticipantRole_READER,
	}}
	if _, err := ReencryptSOState(ctx, le, sfs, sourceID, readable, sourcePriv, "actor-not-readable-destination", destinationPriv, notReadableDestination); err == nil {
		t.Fatal("destination signing actor without read authority was accepted")
	}
}
