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

func TestSpaceResourceChatSenderUsesMountedSessionPeer(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	const channelKey = "chat/channel/space-resource"
	createSpaceResourceChatChannel(t, ctx, tb.WorldState, channelKey)

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

	body := &spaceResourceChatBody{
		engine:   tb.BusEngine,
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
	}
	resources := newSpaceRecordingResourceClient(ctx)
	ctx = resource_server.WithResourceClientContext(ctx, resources)

	invalidSpace := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, "not-a-peer-id")
	invalidClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, invalidSpace.GetMux()))
	if _, err := invalidClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{}); err == nil {
		t.Fatal("AccessWorld accepted an invalid mounted peer ID")
	}

	peerA := tb.Volume.GetPeerID()
	peerBPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := peer.IDFromPrivateKey(peerBPriv)
	if err != nil {
		t.Fatal(err)
	}

	anonymousSpace := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, "")
	anonymousClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, anonymousSpace.GetMux()))
	anonymousWorld, err := anonymousClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(anonymous): %v", err)
	}
	anonymousTyped := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, anonymousWorld.GetResourceId()))
	anonymousCtx := objecttype.WithSessionPeerID(ctx, peerA)
	anonymousChannel, err := anonymousTyped.AccessTypedObject(anonymousCtx, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(anonymous): %v", err)
	}
	anonymousChat := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, anonymousChannel.GetResourceId()))
	if _, err := anonymousChat.SendMessage(anonymousCtx, &spacewave_chat_rpc.SendMessageRequest{Text: "anonymous"}); err == nil || !strings.Contains(err.Error(), spacewave_chat.ErrChatAuthorIdentityRequired.Error()) {
		t.Fatalf("anonymous SendMessage error = %v, want %v", err, spacewave_chat.ErrChatAuthorIdentityRequired)
	}
	resources.ReleaseResource(anonymousChannel.GetResourceId())
	select {
	case <-factory.cleanupCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for anonymous factory cleanup")
	}
	spaceA := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peerA.String())
	spaceB := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peerB.String())

	spaceAClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceA.GetMux()))
	spaceBClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceB.GetMux()))

	worldA, err := spaceAClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(A): %v", err)
	}
	worldB, err := spaceBClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(B): %v", err)
	}

	engineA := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, worldA.GetResourceId()))
	engineB := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, worldB.GetResourceId()))

	// A downstream request carrying B's peer cannot override A's mounted identity.
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

	resources.ReleaseResource(typedA.GetResourceId())
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 1 {
		t.Fatalf("factory cleanups after first A release = %d, want 1", cleanups)
	}
	resources.ReleaseResource(typedAAgain.GetResourceId())
	resources.ReleaseResource(typedB.GetResourceId())
	for range 2 {
		select {
		case <-factory.cleanupCh:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for A/B factory cleanup")
		}
	}
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 3 {
		t.Fatalf("factory cleanups after A/B release = %d, want 3", cleanups)
	}
}

func TestTypedObjectResourceCacheSeparatesPeerIdentities(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	const channelKey = "chat/channel/unbound-owner"
	createSpaceResourceChatChannel(t, ctx, tb.WorldState, channelKey)

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

	peerA := tb.Volume.GetPeerID()
	peerBPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := peer.IDFromPrivateKey(peerBPriv)
	if err != nil {
		t.Fatal(err)
	}

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
	typedB1, err := typedClient.AccessTypedObject(ctxB, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(B1): %v", err)
	}
	typedB2, err := typedClient.AccessTypedObject(ctxB, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(B2): %v", err)
	}

	chatA := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, typedA1.GetResourceId()))
	sendA, err := chatA.SendMessage(ctxA, &spacewave_chat_rpc.SendMessageRequest{Text: "owner A"})
	if err != nil {
		t.Fatalf("SendMessage(A): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, sendA.GetMessageKey(), peerA.String())
	chatB := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, typedB1.GetResourceId()))
	sendB, err := chatB.SendMessage(ctxB, &spacewave_chat_rpc.SendMessageRequest{Text: "owner B"})
	if err != nil {
		t.Fatalf("SendMessage(B): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, sendB.GetMessageKey(), peerB.String())

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
	factory.mu.Lock()
	cleanups = len(factory.cleanups)
	factory.mu.Unlock()
	if cleanups != 2 {
		t.Fatalf("owner cleanups after final releases = %d, want 2", cleanups)
	}
}

func TestSpaceResourceChatSenderPropagatesThroughChildWorldResources(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	const channelKey = "chat/channel/child-world-resources"
	createSpaceResourceChatChannel(t, ctx, tb.WorldState, channelKey)
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

	peerA := tb.Volume.GetPeerID()
	peerBPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := peer.IDFromPrivateKey(peerBPriv)
	if err != nil {
		t.Fatal(err)
	}
	resources := newSpaceRecordingResourceClient(ctx)
	ctx = resource_server.WithResourceClientContext(ctx, resources)
	body := &spaceResourceChatBody{
		engine:   tb.BusEngine,
		engineID: tb.EngineID,
		bucketID: tb.EngineBucketID,
	}
	spaceA := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peerA.String())
	spaceB := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, peerB.String())
	spaceAClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceA.GetMux()))
	spaceBClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, spaceB.GetMux()))
	worldA, err := spaceAClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(A): %v", err)
	}
	worldB, err := spaceBClient.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		t.Fatalf("AccessWorld(B): %v", err)
	}

	engineARaw := resources.client(t, worldA.GetResourceId())
	engineBRaw := resources.client(t, worldB.GetResourceId())
	engineA := s4wave_world.NewSRPCEngineResourceServiceClient(engineARaw)
	engineBWatch := s4wave_world.NewSRPCWatchWorldStateResourceServiceClient(engineBRaw)
	ctxWithPeerB := objecttype.WithSessionPeerID(ctx, peerB)
	ctxWithPeerA := objecttype.WithSessionPeerID(ctx, peerA)

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
	txChat := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, txChannel.GetResourceId()))
	txSend, err := txChat.SendMessage(ctxWithPeerB, &spacewave_chat_rpc.SendMessageRequest{Text: "from A tx"})
	if err != nil {
		t.Fatalf("SendMessage(A tx): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, txSend.GetMessageKey(), peerA.String())
	txService := s4wave_world.NewSRPCTxResourceServiceClient(txRaw)
	if _, err := txService.Discard(ctxWithPeerB, &s4wave_world.DiscardRequest{}); err != nil {
		t.Fatalf("Discard(A tx): %v", err)
	}

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
	trackedTyped := s4wave_world.NewSRPCTypedObjectResourceServiceClient(resources.client(t, tracked.GetResourceId()))
	trackedChannel, err := trackedTyped.AccessTypedObject(ctxWithPeerA, &s4wave_world.AccessTypedObjectRequest{ObjectKey: channelKey})
	if err != nil {
		t.Fatalf("AccessTypedObject(B tracked): %v", err)
	}
	trackedChat := spacewave_chat_rpc.NewSRPCChatResourceServiceClient(resources.client(t, trackedChannel.GetResourceId()))
	cancelWatch()
	_ = watch.Close()
	trackedSend, err := trackedChat.SendMessage(ctxWithPeerA, &spacewave_chat_rpc.SendMessageRequest{Text: "from B tracked"})
	if err != nil {
		t.Fatalf("SendMessage(B tracked): %v", err)
	}
	assertSpaceChatSender(t, ctx, tb.BusEngine, trackedSend.GetMessageKey(), peerB.String())
}

type spaceResourceChatBody struct {
	engine   world.Engine
	engineID string
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
	invoker, cleanup, err := f.base(ctx, le, b, engine, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}
	f.mu.Lock()
	handleID := len(f.handles) + 1
	f.peers = append(f.peers, objecttype.SessionPeerIDFromContext(ctx))
	f.handles = append(f.handles, handleID)
	f.mu.Unlock()
	return invoker, func() {
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
	t.Helper()
	_, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(&spacewave_chat.ChatChannel{Name: "General", CreatedAt: timestamppb.Now()}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", key, err)
	}
	if err := world_types.SetObjectType(ctx, ws, key, spacewave_chat.ChatChannelTypeID); err != nil {
		t.Fatalf("SetObjectType(%s): %v", key, err)
	}
}

func assertSpaceChatSender(t *testing.T, ctx context.Context, engine world.Engine, messageKey, want string) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	defer tx.Discard()
	obj, found, err := tx.GetObject(ctx, messageKey)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", messageKey, err)
	}
	if !found {
		t.Fatalf("message %s not found", messageKey)
	}
	var message *spacewave_chat.ChatMessage
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		message, err = block.UnmarshalBlock[*spacewave_chat.ChatMessage](ctx, bcs, spacewave_chat.NewChatMessageBlock)
		return err
	})
	if err != nil {
		t.Fatalf("UnmarshalBlock(%s): %v", messageKey, err)
	}
	if got := message.GetSenderPeerId(); got != want {
		t.Fatalf("message %s sender = %q, want %q", messageKey, got, want)
	}
}

type spaceRecordingResourceClient struct {
	ctx      context.Context
	mu       sync.Mutex
	nextID   uint32
	muxes    map[uint32]srpc.Invoker
	values   map[uint32]any
	releases map[uint32]func()
}

func newSpaceRecordingResourceClient(ctx context.Context) *spaceRecordingResourceClient {
	return &spaceRecordingResourceClient{
		ctx:      ctx,
		muxes:    make(map[uint32]srpc.Invoker),
		values:   make(map[uint32]any),
		releases: make(map[uint32]func()),
	}
}

func (c *spaceRecordingResourceClient) Context() context.Context {
	return c.ctx
}

func (c *spaceRecordingResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *spaceRecordingResourceClient) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	c.muxes[c.nextID] = mux
	c.values[c.nextID] = value
	c.releases[c.nextID] = releaseFn
	return c.nextID, nil
}

func (c *spaceRecordingResourceClient) ReleaseResource(resourceID uint32) bool {
	c.mu.Lock()
	releaseFn, ok := c.releases[resourceID]
	if ok {
		delete(c.muxes, resourceID)
		delete(c.values, resourceID)
		delete(c.releases, resourceID)
	}
	c.mu.Unlock()
	if !ok {
		return false
	}
	if releaseFn != nil {
		releaseFn()
	}
	return true
}

func (c *spaceRecordingResourceClient) GetResourceValue(resourceID uint32) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[resourceID]
	if !ok {
		return nil, errors.New("resource value not found")
	}
	return value, nil
}

func (c *spaceRecordingResourceClient) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	c.mu.Lock()
	mux := c.muxes[resourceID]
	c.mu.Unlock()
	if mux == nil {
		return nil, errors.New("resource mux not found")
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))), nil
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
