//go:build !js

package wasm

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_rpc_frame "github.com/s4wave/spacewave/net/signaling/rpc/frame"
	"github.com/sirupsen/logrus"
)

// TestE2ESignalingReconnect exchanges signed messages before and after both clients restart.
func TestE2ESignalingReconnect(t *testing.T) {
	// Preserve peer identities while replacing each connection generation.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	endpoint, stop, err := startE2ECloudAuthConfigEndpoint("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	keys := make([]crypto.PrivKey, 2)
	ids := make([]peer.ID, 2)
	for i := range keys {
		keys[i], _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		ids[i], err = peer.IDFromPrivateKey(keys[i])
		if err != nil {
			t.Fatal(err)
		}
	}

	// Each generation reopens the actual WebSocket, frame, and signaling RPC path.
	for range 2 {
		func() {
			clients := make([]*signaling_client.Client, 2)
			for i := range clients {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/signal/ticket", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("X-Peer-ID", ids[i].String())
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr != nil {
					t.Fatal(readErr)
				}
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("ticket status: %d", resp.StatusCode)
				}
				ticket := &api.SignalTicketResponse{}
				if err := ticket.UnmarshalVT(data); err != nil {
					t.Fatal(err)
				}

				// Connect using the ticket returned by the fixture.
				conn, _, err := websocket.Dial(ctx, strings.Replace(endpoint, "http:", "ws:", 1)+"/api/signal/ws?tk="+url.QueryEscape(ticket.GetToken()), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.CloseNow()
				frames := signaling_rpc_frame.NewConn(ctx, conn)
				go func() { _ = frames.ReadPump(nil) }()
				clients[i], err = signaling_client.NewClient(logrus.NewEntry(logrus.New()), signaling_rpc.NewSRPCSignalingClient(srpc.NewClient(frames.OpenStream)), keys[i], nil)
				if err != nil {
					t.Fatal(err)
				}
				defer clients[i].ClearContext()
				clients[i].SetContext(ctx)
			}

			// Rendezvous must forward and acknowledge a signed session message.
			a := clients[0].AddPeerRef(ids[1].String())
			defer a.Release()
			b := clients[1].AddPeerRef(ids[0].String())
			defer b.Release()
			sent := make(chan error, 1)
			go func() {
				_, err := a.Send(ctx, []byte("after reconnect"))
				sent <- err
			}()
			msg, err := b.Recv(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(msg.GetSignedMsg().GetData()); got != "after reconnect" {
				t.Fatalf("received %q", got)
			}
			if err := <-sent; err != nil {
				t.Fatal(err)
			}
		}()
	}
}
