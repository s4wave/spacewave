package resource_space

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	spacewave_chat_world "github.com/s4wave/spacewave/sdk/chat/world"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func spaceResourceClient(t *testing.T, mux srpc.Invoker) srpc.Client {
	t.Helper()
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
}

// TestSpaceResourceChatSenderUsesWorldSigner checks that a Space's chat sends
// carry its World signer, whatever session peer the request names.
func TestSpaceResourceChatSenderUsesWorldSigner(t *testing.T) {
	// Initialize the testbed and channel object.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the chat channel object.
	const channelKey = "chat/channel/space-resource"
	createSpaceResourceChatChannel(t, ctx, tb.WorldState, channelKey)

	// Install a recording object-type factory.
	factory := &recordingChatFactory{
		base:      spacewave_chat_world.ChatChannelType.GetFactory(),
		cleanupCh: make(chan int, 4),
	}
	chatType := objecttype.NewObjectType(spacewave_chat.ChatChannelTypeID, factory.create)
	lookup := func(_ context.Context, typeID string) (objecttype.ObjectType, error) {
		if typeID == spacewave_chat.ChatChannelTypeID {
			return chatType, nil
		}
		return nil, nil
	}
	objectTypeCtrl := objecttype_controller.NewController(lookup)
	objectTypeRelease, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(objectTypeRelease)

	// Present the testbed World without an operation author.
	body := &spaceResourceChatBody{
		engine:   tb.BusEngine,
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
	}

	// Exercise anonymous access and cleanup.
	resources := newSpaceRecordingResourceClient(ctx)
	ctx = resource_server.WithResourceClientContext(ctx, resources)

	// Reject an invalid mounted peer ID.
	invalidSpace := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, "not-a-peer-id")
	invalidClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, invalidSpace.GetMux()))
	if _, err := invalidClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{}); err == nil {
		t.Fatal("AccessWorld accepted an invalid mounted peer ID")
	}

	// Generate a second signing peer.
	peerA := tb.Volume.GetPeerID()
	peerBPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := peer.IDFromPrivateKey(peerBPriv)
	if err != nil {
		t.Fatal(err)
	}

	// Access the World through an anonymous Space.
	anonymousSpace := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, "")
	anonymousClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, anonymousSpace.GetMux()))
	anonymousWorld, err := anonymousClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(anonymous): %v", err)
	}

	// Open the channel through the anonymous World.
	anonymousTyped := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, anonymousWorld.GetResourceId()))
	anonymousCtx := objecttype.WithSessionPeerID(ctx, peerA)
	anonymousChannel, err := anonymousTyped.AccessTypedObject(anonymousCtx, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(anonymous): %v", err)
	}

	// Verify an anonymous send requires an author identity.
	anonymousChat := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, anonymousChannel.GetResourceId()))
	if _, err := anonymousChat.SendMessage(anonymousCtx, &spacewave_chat_rpc.SendMessageRequest{Text: "anonymous"}); err == nil || !strings.Contains(err.Error(), spacewave_chat.ErrChatAuthorIdentityRequired.Error()) {
		t.Fatalf("anonymous SendMessage error = %v, want %v", err, spacewave_chat.ErrChatAuthorIdentityRequired)
	}

	// Release the anonymous channel and wait for its factory cleanup.
	resources.ReleaseResource(anonymousChannel.GetResourceId())
	select {
	case <-factory.cleanupCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for anonymous factory cleanup")
	}

	// Mount one Space per signing peer and resolve typed channels.
	spaceA := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, newSignerChatBody(tb, peerA), peerA.String())
	spaceB := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, newSignerChatBody(tb, peerB), peerB.String())

	// Connect a client to each Space.
	spaceAClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceA.GetMux()))
	spaceBClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceB.GetMux()))

	// Access the World of each Space.
	worldA, err := spaceAClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(A): %v", err)
	}
	worldB, err := spaceBClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(B): %v", err)
	}

	// Open a typed-object client on each World.
	engineA := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, worldA.GetResourceId()))
	engineB := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, worldB.GetResourceId()))

	// Verify the World signer overrides the request's session peer.
	ctxWithPeerB := objecttype.WithSessionPeerID(ctx, peerB)
	typedA, err := engineA.AccessTypedObject(ctxWithPeerB, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(A): %v", err)
	}
	chatA := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, typedA.GetResourceId()))
	sendA, err := chatA.SendMessage(ctxWithPeerB, &spacewave_chat_rpc.SendMessageRequest{Text: "from A"})
	if err != nil {
		t.Fatalf("SendMessage(A): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, sendA.GetMessageKey(), peerA.String())

	// Verify the second mounted peer identity.
	ctxWithPeerA := objecttype.WithSessionPeerID(ctx, peerA)
	typedB, err := engineB.AccessTypedObject(ctxWithPeerA, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(B): %v", err)
	}
	chatB := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, typedB.GetResourceId()))
	sendB, err := chatB.SendMessage(ctxWithPeerA, &spacewave_chat_rpc.SendMessageRequest{Text: "from B"})
	if err != nil {
		t.Fatalf("SendMessage(B): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, sendB.GetMessageKey(), peerB.String())

	// Verify one factory handle per peer after the anonymous open.
	factory.mu.Lock()
	if got := len(factory.peers); got != 3 {
		factory.mu.Unlock()
		t.Fatalf("factory opens = %d, want 3", got)
	}
	if factory.peers[0] != "" || factory.peers[1] != peerA || factory.peers[2] != peerB {
		got := append([]peer.ID(nil), factory.peers...)
		factory.mu.Unlock()
		t.Fatalf("factory peers = %v, want [anonymous %s %s]", got, peerA, peerB)
	}
	if factory.handles[1] == factory.handles[2] {
		factory.mu.Unlock()
		t.Fatal("A and B used the same recording-factory handle")
	}
	factory.mu.Unlock()

	// Keep A live while reacquiring it. The keyed owner must reuse A without
	// opening a fourth factory handle or changing B's handle.
	typedAAgain, err := engineA.AccessTypedObject(ctxWithPeerB, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(A again): %v", err)
	}
	if typedAAgain.GetResourceId() == typedA.GetResourceId() {
		t.Fatal("reacquiring A returned the same child resource ID")
	}

	// Verify the reacquire opened and cleaned no handle.
	factory.mu.Lock()
	opens := len(factory.peers)
	cleanups := len(factory.cleanups)
	factory.mu.Unlock()
	if opens != 3 {
		t.Fatalf("factory opens after A reacquire = %d, want 3", opens)
	}
	if cleanups != 1 {
		t.Fatalf("factory cleanups while A and B are live = %d, want 1", cleanups)
	}

	// Release one A reference while the other keeps A open.
	resources.ReleaseResource(typedA.GetResourceId())
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 1 {
		t.Fatalf("factory cleanups after first A release = %d, want 1", cleanups)
	}

	// Release the remaining references and wait for both cleanups.
	resources.ReleaseResource(typedAAgain.GetResourceId())
	resources.ReleaseResource(typedB.GetResourceId())
	for range 2 {
		select {
		case <-factory.cleanupCh:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for A/B factory cleanup")
		}
	}

	// Verify every handle was cleaned up.
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 3 {
		t.Fatalf("factory cleanups after A/B release = %d, want 3", cleanups)
	}
}

func TestTypedObjectResourceCacheSeparatesPeerIdentities(t *testing.T) {
	// Start a World testbed for peer-specific typed Resource caching.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the chat channel shared by both signing peers.
	const channelKey = "chat/channel/unbound-owner"
	createSpaceResourceChatChannel(t, ctx, tb.WorldState, channelKey)

	// Register a chat factory that records peer-specific handles and cleanup.
	factory := &recordingChatFactory{
		base:      spacewave_chat_world.ChatChannelType.GetFactory(),
		cleanupCh: make(chan int, 2),
	}
	chatType := objecttype.NewObjectType(spacewave_chat.ChatChannelTypeID, factory.create)
	objectTypeCtrl := objecttype_controller.NewController(func(_ context.Context, typeID string) (objecttype.ObjectType, error) {
		if typeID == spacewave_chat.ChatChannelTypeID {
			return chatType, nil
		}
		return nil, nil
	})
	objectTypeRelease, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(objectTypeRelease)

	// Generate a second peer identity distinct from the test volume signer.
	peerA := tb.Volume.GetPeerID()
	peerBPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := peer.IDFromPrivateKey(peerBPriv)
	if err != nil {
		t.Fatal(err)
	}

	// Expose typed World Resources through a recording Resource client.
	resources := newSpaceRecordingResourceClient(ctx)
	ctx = resource_server.WithResourceClientContext(ctx, resources)
	owner := resource_world.NewTypedObjectResource(
		tb.Logger,
		tb.Bus,
		world.NewEngineWorldState(tb.BusEngine, true),
		tb.BusEngine,
	)
	t.Cleanup(owner.Close)
	mux := resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_world.SRPCRegisterTypedObjectResourceService(mux, owner)
	})
	typedClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(spaceResourceClient(t, mux))

	// Open two references to the chat channel for each peer.
	ctxA := objecttype.WithSessionPeerID(ctx, peerA)
	ctxB := objecttype.WithSessionPeerID(ctx, peerB)
	typedA1, err := typedClient.AccessTypedObject(ctxA, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(A1): %v", err)
	}
	typedA2, err := typedClient.AccessTypedObject(ctxA, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(A2): %v", err)
	}

	// Open the second peer references to the same chat channel.
	typedB1, err := typedClient.AccessTypedObject(ctxB, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(B1): %v", err)
	}
	typedB2, err := typedClient.AccessTypedObject(ctxB, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(B2): %v", err)
	}

	// Verify both peer handles stay distinct and live while their references remain.
	factory.mu.Lock()
	opens := len(factory.peers)
	distinct := len(factory.handles) == 2 && factory.handles[0] != factory.handles[1]
	cleanups := len(factory.cleanups)
	factory.mu.Unlock()
	if opens != 2 || !distinct {
		t.Fatalf("owner factory opens = %d handles=%v, want two distinct handles", opens, distinct)
	}
	if cleanups != 0 {
		t.Fatalf("owner factory cleanups while all refs live = %d, want 0", cleanups)
	}

	// Release the first peer references and verify cleanup occurs after its last reference.
	resources.ReleaseResource(typedA1.GetResourceId())
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 0 {
		t.Fatalf("owner cleanups after first A release = %d, want 0", cleanups)
	}
	resources.ReleaseResource(typedA2.GetResourceId())
	select {
	case <-factory.cleanupCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for A cleanup")
	}

	// Release the second peer references and verify cleanup occurs after its last reference.
	resources.ReleaseResource(typedB1.GetResourceId())
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 1 {
		t.Fatalf("owner cleanups after first B release = %d, want 1", cleanups)
	}
	resources.ReleaseResource(typedB2.GetResourceId())
	select {
	case <-factory.cleanupCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for B cleanup")
	}

	// Verify both peer handles were cleaned after their final references.
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 2 {
		t.Fatalf("owner cleanups after final releases = %d, want 2", cleanups)
	}
}

func TestSpaceResourceChatSenderPropagatesThroughChildWorldResources(t *testing.T) {
	// Initialize the testbed.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the chat channel object.
	const channelKey = "chat/channel/child-world-resources"
	createSpaceResourceChatChannel(t, ctx, tb.WorldState, channelKey)

	// Install a recording object-type factory.
	factory := &recordingChatFactory{
		base:      spacewave_chat_world.ChatChannelType.GetFactory(),
		cleanupCh: make(chan int, 4),
	}
	chatType := objecttype.NewObjectType(spacewave_chat.ChatChannelTypeID, factory.create)
	objectTypeCtrl := objecttype_controller.NewController(func(_ context.Context, typeID string) (objecttype.ObjectType, error) {
		if typeID == spacewave_chat.ChatChannelTypeID {
			return chatType, nil
		}
		return nil, nil
	})
	objectTypeRelease, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(objectTypeRelease)

	// Generate a second signing peer.
	peerA := tb.Volume.GetPeerID()
	peerBPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := peer.IDFromPrivateKey(peerBPriv)
	if err != nil {
		t.Fatal(err)
	}

	// Record the child resources the Spaces create.
	resources := newSpaceRecordingResourceClient(ctx)
	ctx = resource_server.WithResourceClientContext(ctx, resources)

	// Mount one Space per signing peer.
	spaceA := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, newSignerChatBody(tb, peerA), peerA.String())
	spaceB := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, newSignerChatBody(tb, peerB), peerB.String())
	spaceAClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceA.GetMux()))
	spaceBClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceB.GetMux()))

	// Access the World of each Space.
	worldA, err := spaceAClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(A): %v", err)
	}
	worldB, err := spaceBClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(B): %v", err)
	}

	// Open the engine of A and the World-state watch of B, naming the other peer in each request.
	engineARaw := resources.client(t, worldA.GetResourceId())
	engineBRaw := resources.client(t, worldB.GetResourceId())
	engineA := s4wave_world.NewSRPCEngineResourceServiceClient(engineARaw)
	engineBWatch := s4wave_world.NewSRPCWatchWorldStateResourceServiceClient(engineBRaw)
	ctxWithPeerB := objecttype.WithSessionPeerID(ctx, peerB)
	ctxWithPeerA := objecttype.WithSessionPeerID(ctx, peerA)

	// Open the channel in a read transaction on A.
	txResp, err := engineA.NewTransaction(ctxWithPeerB, &s4wave_world.NewTransactionRequest{Write: false})
	if err != nil {
		t.Fatalf("NewTransaction(A): %v", err)
	}
	txRaw := resources.client(t, txResp.GetResourceId())
	txTyped := s4wave_world.NewSRPCTypedObjectResourceServiceClient(txRaw)
	txChannel, err := txTyped.AccessTypedObject(ctxWithPeerB, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(A tx): %v", err)
	}

	// Verify a send through the transaction carries the signer of A.
	txChat := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, txChannel.GetResourceId()))
	txSend, err := txChat.SendMessage(ctxWithPeerB, &spacewave_chat_rpc.SendMessageRequest{Text: "from A tx"})
	if err != nil {
		t.Fatalf("SendMessage(A tx): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, txSend.GetMessageKey(), peerA.String())

	// Discard the transaction.
	txService := s4wave_world.NewSRPCTxResourceServiceClient(txRaw)
	if _, err := txService.Discard(ctxWithPeerB, &s4wave_world.DiscardRequest{}); err != nil {
		t.Fatalf("Discard(A tx): %v", err)
	}

	// Watch the World state of B and take its first tracked state.
	watchCtx, cancelWatch := context.WithCancel(ctxWithPeerA)
	watch, err := engineBWatch.WatchWorldState(watchCtx, &s4wave_world.WatchWorldStateRequest{})
	if err != nil {
		t.Fatalf("WatchWorldState(B): %v", err)
	}
	defer cancelWatch()
	tracked, err := watch.Recv()
	if err != nil {
		t.Fatalf("WatchWorldState(B) Recv: %v", err)
	}

	// Open the channel through the tracked state.
	trackedTyped := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, tracked.GetResourceId()))
	trackedChannel, err := trackedTyped.AccessTypedObject(ctxWithPeerA, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(B tracked): %v", err)
	}
	trackedChat := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, trackedChannel.GetResourceId()))

	// Close the watch; the channel resource stays open.
	cancelWatch()
	_ = watch.Close()

	// Verify a send through the tracked state carries the signer of B.
	trackedSend, err := trackedChat.SendMessage(ctxWithPeerA, &spacewave_chat_rpc.SendMessageRequest{Text: "from B tracked"})
	if err != nil {
		t.Fatalf("SendMessage(B tracked): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, trackedSend.GetMessageKey(), peerB.String())
}

// newSignerChatBody presents the testbed World under one signing device, as
// each session's mounted SharedObject engine does.
func newSignerChatBody(tb *testbed.Testbed, device peer.ID) *spaceResourceChatBody {
	return &spaceResourceChatBody{
		engine:   &signerEngine{Engine: tb.BusEngine, device: device},
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
	}
}

// signerEngine is a World engine whose operations one device signs.
type signerEngine struct {
	world.Engine
	// device signs every operation and is its own person.
	device peer.ID
}

// OperationAuthor returns the signing device as both device and person.
func (e *signerEngine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return e.device, e.device.String(), nil
}

// spaceResourceChatBody presents one World engine as a mounted Space body.
type spaceResourceChatBody struct {
	// engine is the mounted Space World.
	engine world.Engine
	// engineID names that World on the bus.
	engineID string
	// bucketID names the World's bucket.
	bucketID string
}

func (b *spaceResourceChatBody) GetWorldEngine() world.Engine {
	return b.engine
}

func (b *spaceResourceChatBody) GetWorldEngineID() string {
	return b.engineID
}

func (b *spaceResourceChatBody) GetWorldEngineBucketID() string {
	return b.bucketID
}

func (b *spaceResourceChatBody) GetSharedObjectRef() *sobject.SharedObjectRef {
	return nil
}

func (b *spaceResourceChatBody) GetSharedObject() sobject.SharedObject {
	return nil
}

var _ space.SpaceSharedObjectBody = (*spaceResourceChatBody)(nil)

type recordingChatFactory struct {
	base objecttype.ObjectTypeFactory

	mu        sync.Mutex
	peers     []peer.ID
	handles   []int
	cleanups  []int
	cleanupCh chan int
}

func (f *recordingChatFactory) create(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	// Create the chat Resource and record the peer identity of its handle.
	invoker, cleanup, err := f.base(ctx, le, b, engine, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}

	// Record the opened chat handle under the factory lock.
	f.mu.Lock()
	handleID := len(f.handles) + 1
	f.peers = append(f.peers, objecttype.SessionPeerIDFromContext(ctx))
	f.handles = append(f.handles, handleID)
	f.mu.Unlock()
	return invoker, func() {
		// Record the chat handle cleanup and release its underlying Resource.
		f.mu.Lock()
		f.cleanups = append(f.cleanups, handleID)
		f.mu.Unlock()
		if f.cleanupCh != nil {
			f.cleanupCh <- handleID
		}
		if cleanup != nil {
			cleanup()
		}
	}, nil
}

func createSpaceResourceChatChannel(t *testing.T, ctx context.Context, ws world.WorldState, key string) {
	// Create a typed chat channel in the World for Resource tests.
	t.Helper()
	createdObject, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(&spacewave_chat.ChatChannel{Name: "General", CreatedAt: timestamppb.Now()}, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", key, err)
	}
	if err := world_types.SetObjectType(ctx, ws, key, spacewave_chat.ChatChannelTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", key, err)
	}
}

func assertSpaceChatSender(t *testing.T, ctx context.Context, engine world.Engine, messageKey, want string) {
	// Read the saved chat message and verify its signing peer.
	t.Helper()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	defer tx.Discard()

	// Acquire the saved chat message from the World snapshot.
	obj, found, err := tx.GetObject(ctx, messageKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", messageKey, err)
	}
	if !found {
		t.Fatalf("message %s not found", messageKey)
	}

	// Decode the saved chat message body.
	var message *spacewave_chat.ChatMessage
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		message, err = block.UnmarshalBlock[*spacewave_chat.ChatMessage](ctx, bcs, spacewave_chat.NewChatMessageBlock)
		return err
	})
	if err != nil {
		t.Fatalf("UnmarshalBlock(%s): %v", messageKey, err)
	}

	// Verify the chat message carries the expected signer.
	if got := message.GetSenderPeerId(); got != want {
		t.Fatalf("message %s sender = %q, want %q", messageKey, got, want)
	}
}

type spaceAttachedResourceClient struct {
	// Client forwards calls to the attached resource.
	srpc.Client
	// done closes when the attached resource is released.
	done <-chan struct{}
}

func (c *spaceAttachedResourceClient) Done() <-chan struct{} {
	return c.done
}

type spaceRecordingResourceClient struct {
	ctx      context.Context
	mu       sync.Mutex
	nextID   uint32
	muxes    map[uint32]srpc.Invoker
	values   map[uint32]any
	releases map[uint32]func()
	// dones is guarded by mu and closes when its resource is released.
	dones map[uint32]chan struct{}
}

func newSpaceRecordingResourceClient(ctx context.Context) *spaceRecordingResourceClient {
	return &spaceRecordingResourceClient{
		ctx:      ctx,
		muxes:    make(map[uint32]srpc.Invoker),
		values:   make(map[uint32]any),
		releases: make(map[uint32]func()),
		dones:    make(map[uint32]chan struct{}),
	}
}

func (c *spaceRecordingResourceClient) Context() context.Context {
	return c.ctx
}

func (c *spaceRecordingResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *spaceRecordingResourceClient) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	// Register the Resource value, route, release callback, and lifetime under one ID.
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	c.muxes[c.nextID] = mux
	c.values[c.nextID] = value
	c.releases[c.nextID] = releaseFn
	c.dones[c.nextID] = make(chan struct{})
	return c.nextID, nil
}

func (c *spaceRecordingResourceClient) ReleaseResource(resourceID uint32) bool {
	// Remove the recorded Resource before ending its attachment lifetime.
	c.mu.Lock()
	releaseFn, ok := c.releases[resourceID]
	done := c.dones[resourceID]
	if ok {
		delete(c.muxes, resourceID)
		delete(c.values, resourceID)
		delete(c.releases, resourceID)
		delete(c.dones, resourceID)
	}
	c.mu.Unlock()
	if !ok {
		return false
	}

	// Notify attachment callers and release the underlying Resource.
	close(done)
	if releaseFn != nil {
		releaseFn()
	}
	return true
}

func (c *spaceRecordingResourceClient) GetResourceValue(resourceID uint32) (any, error) {
	// Read a recorded Resource value under the client lock.
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[resourceID]
	if !ok {
		return nil, errors.New("resource value not found")
	}
	return value, nil
}

func (c *spaceRecordingResourceClient) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	// Resolve the recorded Resource route and its attachment lifetime.
	c.mu.Lock()
	mux := c.muxes[resourceID]
	done := c.dones[resourceID]
	c.mu.Unlock()
	if mux == nil {
		return nil, errors.New("resource mux not found")
	}
	return &spaceAttachedResourceClient{
		Client: srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))),
		done:   done,
	}, nil
}

func (c *spaceRecordingResourceClient) client(t *testing.T, resourceID uint32) srpc.Client {
	t.Helper()
	client, err := c.GetAttachedResource(resourceID)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

var _ resource_server.ResourceClientContext = (*spaceRecordingResourceClient)(nil)
