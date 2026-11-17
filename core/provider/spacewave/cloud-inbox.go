package provider_spacewave

import (
	"context"
	"net/url"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/cloudinbox"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	flowgraph_eventsource "github.com/s4wave/spacewave/sdk/flowgraph/eventsource"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// CloudInboxType is the object type of a cloud inbox event source. Its
// resource reads the inbox as the account that owns the object's Space,
// signing each request with that account's session key, so no plugin needs a
// signing grant.
var CloudInboxType = objecttype.NewObjectType(cloudinbox.TypeID, cloudInboxFactory)

// cloudInboxFactory serves EventSourceService for a cloud inbox object. It
// reads the inbox ID from the object's body and the account from the Space's
// engine ID; it reaches the account only when the run reads the source.
func cloudInboxFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	wst world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	// Read the inbox ID from the object's body.
	if wst == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	inboxID, err := readCloudInboxID(ctx, wst, objectKey)
	if err != nil {
		return nil, nil, err
	}

	// Name the account that owns the Space.
	providerID, accountID, _, err := space.ParseSpaceEngineId(objecttype.EngineIDFromContext(ctx))
	if err != nil {
		return nil, nil, errors.Wrap(err, "cloud inbox needs a Space")
	}

	// Serve the source over a transport signed as that account.
	open := func(ctx context.Context) (cloudinbox.Transport, func(), error) {
		return openCloudInbox(ctx, b, providerID, accountID, inboxID)
	}
	mux := srpc.NewMux()
	if err := flowgraph_eventsource.SRPCRegisterEventSourceService(mux, cloudinbox.NewSource(open)); err != nil {
		return nil, nil, err
	}
	return mux, func() {}, nil
}

// readCloudInboxID returns the inbox ID a cloud inbox object's body names.
func readCloudInboxID(ctx context.Context, wst world.WorldState, objectKey string) (string, error) {
	// Read the object's body, holding the object while reading it.
	obj, found, err := wst.GetObject(ctx, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return "", err
	}
	if !found {
		return "", world.ErrObjectNotFound
	}
	var body *cloudinbox.CloudInbox
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		body, err = block.UnmarshalBlock[cloudinbox.CloudInbox](ctx, bcs)
		return err
	})
	if err != nil {
		return "", errors.Wrap(err, "read cloud inbox")
	}

	// Require an inbox to read.
	if body.GetInboxId() == "" {
		return "", errors.New("cloud inbox names no inbox")
	}
	return body.GetInboxId(), nil
}

// openCloudInbox returns the inbox's transport, signed with the account's
// session key. It waits while the account is not yet loaded or its session is
// locked, and fails when the Space's provider is not Spacewave Cloud.
func openCloudInbox(ctx context.Context, b bus.Bus, providerID, accountID, inboxID string) (cloudinbox.Transport, func(), error) {
	// Reach the account.
	acc, ref, err := provider.ExAccessProviderAccount(ctx, b, providerID, accountID, false, nil)
	if err != nil {
		return nil, nil, err
	}
	swAcc, ok := acc.(*ProviderAccount)
	if !ok {
		ref.Release()
		return nil, nil, errors.Errorf("cloud inbox: the Space's provider %q is not Spacewave Cloud", providerID)
	}

	// Wait for a session that can sign.
	cli, err := swAcc.waitSessionClient(ctx)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}
	return &cloudInboxTransport{cli: cli, inboxID: inboxID}, ref.Release, nil
}

// waitSessionClient returns a signing-capable session client, waiting on the
// account broadcast while every session is locked or none is mounted.
func (a *ProviderAccount) waitSessionClient(ctx context.Context) (*SessionClient, error) {
	for {
		// Take the wait channel before the check, so no change falls between.
		var wait <-chan struct{}
		a.accountBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			wait = getWait()
		})
		if cli, _, _, err := a.getReadySessionClient(ctx); err == nil {
			return cli, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}

// cloudInboxTransport is a cloud inbox's transport over a session client.
type cloudInboxTransport struct {
	// cli signs each request with the account's session key.
	cli *SessionClient
	// inboxID is the inbox read.
	inboxID string
}

// List implements cloudinbox.Transport.
func (t *cloudInboxTransport) List(ctx context.Context, since uint64) (*cloudinbox.ListInboxEventsResponse, error) {
	return t.cli.ListInboxEvents(ctx, t.inboxID, since)
}

// Ack implements cloudinbox.Transport.
func (t *cloudInboxTransport) Ack(ctx context.Context, seqno uint64) error {
	return t.cli.AckInbox(ctx, t.inboxID, seqno)
}

// Watch implements cloudinbox.Transport. It pings the channel as the session
// websocket does, so a dead connection fails the read.
func (t *cloudInboxTransport) Watch(ctx context.Context, onHead func(seqno uint64)) error {
	// Trade the signed session for a channel ticket, and open the channel.
	ticket, err := t.cli.GetInboxTicket(ctx, t.inboxID)
	if err != nil {
		return err
	}
	wsPath, err := url.JoinPath(t.cli.baseURL, inboxPath(t.inboxID, "ws"))
	if err != nil {
		return errors.Wrap(err, "build inbox channel URL")
	}
	conn, err := dialSessionWS(ctx, wsPath+"?tk="+url.QueryEscape(ticket))
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	// Ping until the read ends.
	pingCtx, cancelPing := context.WithCancel(ctx)
	defer cancelPing()
	go func() {
		_ = runWebSocketPing(pingCtx, conn, sessionWebSocketPingInterval, sessionWebSocketPingTimeout)
	}()

	// Report each head the Cloud pushes.
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return errors.Wrap(err, "read inbox channel")
		}
		frame := &cloudinbox.WsInboxServerFrame{}
		if err := frame.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "decode inbox channel frame")
		}
		if head := frame.GetHead(); head != nil {
			onHead(head.GetSeqno())
		}
	}
}

// _ is a type assertion
var _ cloudinbox.Transport = (*cloudInboxTransport)(nil)
