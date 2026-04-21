package bifrost_api

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	pubsub_api "github.com/s4wave/spacewave/net/pubsub/api"
)

// acquirePeerTimeout is the timeout for acquiring private key
var acquirePeerTimeout = time.Second * 10

// Subscribe subscribes to a pubsub channel.
//
// TODO: move this code to pubsub/api
func (a *API) Subscribe(serv pubsub_api.SRPCPubSubService_SubscribeStream) error {
	ctx := serv.Context()

	var channelID string
	var handlePeerID peer.ID
	var handlePeer peer.Peer
	var sub pubsub.Subscription
	var handlePeerRef directive.Reference
	defer func() {
		if handlePeerRef != nil {
			handlePeerRef.Release()
		}
	}()
	for {
		msg, err := serv.Recv()
		if err != nil {
			return err
		}
		if msgPrivKey := msg.GetPrivKeyPem(); msgPrivKey != "" {
			if len(handlePeerID) != 0 {
				return errors.New("peer id or private key cannot be specified twice")
			}
			pkey, err := keypem.ParsePrivKeyPem([]byte(msgPrivKey))
			if err != nil {
				return err
			}
			handlePeer, err = peer.NewPeer(pkey)
			if err != nil {
				return err
			}
			handlePeerID, err = peer.IDFromPrivateKey(pkey)
			if err != nil {
				return err
			}
		}
		if msgPeerID := msg.GetPeerId(); msgPeerID != "" && len(msg.GetPrivKeyPem()) == 0 {
			if len(handlePeerID) != 0 {
				return errors.New("peer id cannot be specified twice")
			}
			handlePeerID, err = peer.IDB58Decode(msgPeerID)
			if err != nil {
				return err
			}
			pubCtx, pubCtxCancel := context.WithTimeout(ctx, acquirePeerTimeout)
			handlePeer, _, handlePeerRef, err = peer.GetPeerWithID(pubCtx, a.bus, handlePeerID, false, nil)
			pubCtxCancel()
			if err != nil || handlePeer == nil {
				return errors.Errorf("peer not identified locally: %s", msgPeerID)
			}
		}
		if chid := msg.GetChannelId(); chid != "" {
			if channelID != "" {
				return errors.New("channel id cannot be specified twice")
			}
			if handlePeer == nil || len(handlePeerID) == 0 {
				return errors.New("peer id must be specified before or with channel id")
			}
			channelID = chid
			// acquire channel
			handlePeerPrivKey, err := handlePeer.GetPrivKey(ctx)
			if err != nil {
				return err
			}
			val, _, subRef, err := pubsub.ExBuildChannelSubscription(
				ctx,
				a.bus,
				false,
				channelID,
				handlePeerPrivKey,
				nil,
			)
			if err != nil {
				return err
			}
			defer subRef.Release()
			sub = val
			err = serv.Send(&pubsub_api.SubscribeResponse{
				SubscriptionStatus: &pubsub_api.SubscriptionStatus{
					Subscribed: true,
				},
			})
			if err != nil {
				return err
			}
			// note: the defer call is for releasing the handler.
			defer val.AddHandler(func(m pubsub.Message) {
				go func() {
					_ = serv.Send(&pubsub_api.SubscribeResponse{
						IncomingMessage: &pubsub_api.IncomingMessage{
							FromPeerId:    m.GetFrom().String(),
							Data:          m.GetData(),
							Authenticated: m.GetAuthenticated(),
						},
					})
				}()
			})()
		}
		if channelID == "" || sub == nil {
			return errors.New("channel id must be specified in first message")
		}

		pubReqData := msg.GetPublishRequest().GetData()
		if len(pubReqData) != 0 {
			if err := sub.Publish(pubReqData); err != nil {
				return err
			}
			if mid := msg.GetPublishRequest().GetIdentifier(); mid != 0 {
				err = serv.Send(&pubsub_api.SubscribeResponse{
					OutgoingStatus: &pubsub_api.OutgoingStatus{
						Identifier: mid,
						Sent:       true,
					},
				})
				if err != nil {
					return err
				}
			}
		}
	}
}
