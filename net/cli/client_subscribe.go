package cli

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	pubsub_api "github.com/s4wave/spacewave/net/pubsub/api"
)

// publishTimeout is the timeout to wait for ack of publish
var publishTimeout = time.Second * 30

// RunSubscribe runs the subscription command.
func (a *ClientArgs) RunSubscribe(_ *cli.Context) error {
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	ctx, ctxCancel := context.WithCancel(a.GetContext())
	defer ctxCancel()

	if a.SubscribeConf.GetPeerId() == "" {
		pkdat, privKey, err := a.LoadOrGenerateIdentifyKey()
		if err != nil {
			return err
		}
		pid, err := peer.IDFromPrivateKey(privKey)
		if err != nil {
			return err
		}
		a.SubscribeConf.PeerId = pid.String()
		a.SubscribeConf.PrivKeyPem = string(pkdat)
	}

	client, err := c.Subscribe(ctx)
	if err != nil {
		return err
	}
	if err := client.Send(&a.SubscribeConf); err != nil {
		return err
	}

	// Start publisher loop.
	input := os.Stdin
	errCh := make(chan error, 10)
	publishAckCh := make(chan uint32, 10)
	go func() {
		scan := bufio.NewScanner(input)
		spubMsg := &pubsub_api.SubscribeRequest{}
		pubMsg := &pubsub_api.PublishRequest{}
		spubMsg.PublishRequest = pubMsg
		var sendIdentifier uint32
		var dataRecv bool
		for scan.Scan() {
			line := scan.Text()
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			dataRecv = true
			data, err := base64.StdEncoding.DecodeString(line)
			if err != nil {
				errCh <- errors.Wrap(err, "decode base64 input line")
				return
			}
			sendIdentifier++
			pubMsg.Identifier = sendIdentifier
			pubMsg.Data = data
			if err := client.Send(spubMsg); err != nil {
				errCh <- err
				return
			}
			pubCtx, pubCtxCancel := context.WithTimeout(ctx, publishTimeout)
			var aack uint32
			for aack != pubMsg.Identifier {
				select {
				case <-ctx.Done():
					pubCtxCancel()
					return
				case <-pubCtx.Done():
					pubCtxCancel()
					errCh <- pubCtx.Err()
					return
				case aack = <-publishAckCh:
				}
			}
			pubCtxCancel()
		}
		if dataRecv {
			errCh <- io.EOF
		}
	}()

	recvCh := make(chan *pubsub_api.SubscribeResponse, 10)
	go func() {
		for {
			msg, err := client.Recv()
			if err != nil && err != context.Canceled && err != io.EOF {
				errCh <- err
				return
			}
			select {
			case <-ctx.Done():
				return
			case recvCh <- msg:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err == io.EOF {
				err = nil
			}
			return err
		case msg := <-recvCh:
			/*
				if msg.GetSubscriptionStatus().GetError() != "" {
					if err != context.Canceled && err != io.EOF {
						os.Stderr.WriteString(err.Error())
						os.Stderr.WriteString("\n")
					}
				}
			*/
			if msg.GetSubscriptionStatus().GetSubscribed() {
				os.Stdout.WriteString("sub ")
				os.Stdout.WriteString(a.SubscribeConf.GetChannelId())
				os.Stdout.WriteString("\n")
			}
			if outID := msg.GetOutgoingStatus().GetIdentifier(); outID != 0 {
				os.Stdout.WriteString("out")
				os.Stdout.WriteString(" ")
				os.Stdout.WriteString(
					strconv.Itoa(int(msg.GetOutgoingStatus().GetIdentifier())),
				)
				os.Stdout.WriteString(" ")
				mmsg := "ok"
				if !msg.GetOutgoingStatus().GetSent() {
					mmsg = "nok"
				}
				os.Stdout.WriteString(
					mmsg,
				)
				os.Stdout.WriteString("\n")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case publishAckCh <- outID:
				}
			}
			if len(msg.GetIncomingMessage().GetData()) != 0 {
				os.Stdout.WriteString(msg.GetIncomingMessage().GetFromPeerId())
				os.Stdout.WriteString(" ")
				authedMsg := "authenticated"
				if !msg.GetIncomingMessage().GetAuthenticated() {
					authedMsg = "unverified"
				}
				os.Stdout.WriteString(authedMsg)
				os.Stdout.WriteString(" ")
				os.Stdout.WriteString(
					base64.StdEncoding.EncodeToString(
						msg.GetIncomingMessage().GetData(),
					),
				)
				os.Stdout.WriteString("\n")
			}
		}
	}
}
