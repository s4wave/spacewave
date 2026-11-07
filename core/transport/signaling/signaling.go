// Package signaling serves authenticated signaling tickets and peer sockets.
package signaling

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/transport/reqsig"
	"github.com/s4wave/spacewave/net/peer"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_frame "github.com/s4wave/spacewave/net/signaling/rpc/frame"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/sirupsen/logrus"
)

// ticketLifetime is how long a ticket waits for its socket.
const ticketLifetime = 30 * time.Second

// maxMessage bounds one signaling message.
const maxMessage = 1 << 20

// peerKey names the ticket holder on its socket's RPC context.
type peerKey struct{}

// Admitted reports whether a peer holds a connection that admits it to
// signaling, such as a PC's device socket.
type Admitted func(ctx context.Context, pid peer.ID) bool

// ticket binds a ticket to the peer it was issued to.
type ticket struct {
	// peer is the ticket holder.
	peer peer.ID
	// expires is when the ticket stops admitting a socket.
	expires time.Time
}

// Signaling serves the signaling endpoints. A peer signs a ticket request
// with its key, so only keys already authenticated on a device socket or a
// signed-in peer socket are admitted. The service holds every peer's
// signaling socket, so the server that runs it decides how long exchanges
// survive.
type Signaling struct {
	// signingEnv separates request signatures from those of other services.
	signingEnv string
	// admitted admits peers that hold a connection elsewhere.
	admitted Admitted
	// mux serves the signaling RPCs on every socket.
	mux srpc.Mux

	// mtx guards tickets and peers.
	mtx sync.Mutex
	// tickets holds each unused ticket by its token.
	tickets map[string]ticket
	// peers counts each key's open peer sockets.
	peers map[peer.ID]int
}

// New constructs the signaling service for the peers admitted reports, and
// for the peers holding a peer socket on it. signingEnv must match the
// transport client's request signing environment.
func New(le *logrus.Entry, admitted Admitted, signingEnv string) *Signaling {
	// Name each socket's caller by the peer its ticket was issued to. A fresh
	// mux refuses no registration.
	server := signaling_server.NewServerWithIdentify(le, func(ctx context.Context) (peer.ID, error) {
		return ctx.Value(peerKey{}).(peer.ID), nil
	})
	mux := srpc.NewMux()
	_ = signaling_rpc.SRPCRegisterSignaling(mux, server)
	return &Signaling{signingEnv: signingEnv, admitted: admitted, mux: mux, tickets: make(map[string]ticket), peers: make(map[peer.ID]int)}
}

// admits reports whether pid holds a peer socket here or a connection that
// admitted reports.
func (s *Signaling) admits(ctx context.Context, pid peer.ID) bool {
	s.mtx.Lock()
	held := s.peers[pid] != 0
	s.mtx.Unlock()
	return held || s.admitted(ctx, pid)
}

// ServeTicket issues a ticket to a connected peer that signed the request. A
// refusal is 403, never 401, because the peer retries 403 and gives up on
// 401: a desktop asks for its ticket before its device socket finishes
// connecting.
func (s *Signaling) ServeTicket(w http.ResponseWriter, r *http.Request) {
	// Accept only a signed request from an admitted peer.
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	pid, err := VerifyRequest(r, time.Now(), s.signingEnv)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !s.admits(r.Context(), pid) {
		http.Error(w, "Connect this PC first", http.StatusForbidden)
		return
	}

	// Issue a random single-use token, dropping expired ones.
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(token[:])
	now := time.Now()
	s.mtx.Lock()
	for key, issued := range s.tickets {
		if now.After(issued.expires) {
			delete(s.tickets, key)
		}
	}
	s.tickets[encoded] = ticket{peer: pid, expires: now.Add(ticketLifetime)}
	s.mtx.Unlock()

	// Answer in the binary form the peer decodes.
	data, err := (&api.SignalTicketResponse{Token: encoded}).MarshalVT()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

// ServeSocket redeems a ticket and serves its holder's signaling RPCs until
// the socket closes.
func (s *Signaling) ServeSocket(w http.ResponseWriter, r *http.Request) {
	// Redeem the ticket once.
	token := r.URL.Query().Get("tk")
	s.mtx.Lock()
	issued, ok := s.tickets[token]
	delete(s.tickets, token)
	s.mtx.Unlock()
	if !ok || time.Now().After(issued.expires) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Serve the holder's RPCs, one frame per message, until either side closes.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), peerKey{}, issued.peer))
	defer cancel()
	frames := signaling_rpc_frame.NewConn(ctx, conn)
	_ = frames.ReadPump(signaling_rpc_frame.NewServerAccept(ctx, s.mux))
}

// ServePeer admits the peer that signed r to signaling until its socket
// closes. The caller has checked the request's sign-in, which ties the key to
// an account.
func (s *Signaling) ServePeer(w http.ResponseWriter, r *http.Request) {
	// Accept only a socket the peer's key signed.
	pid, err := VerifyRequest(r, time.Now(), s.signingEnv)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	// Count the key admitted while the socket is open.
	s.mtx.Lock()
	s.peers[pid]++
	s.mtx.Unlock()
	defer func() {
		s.mtx.Lock()
		defer s.mtx.Unlock()
		if s.peers[pid]--; s.peers[pid] == 0 {
			delete(s.peers, pid)
		}
	}()

	// Answer the peer's pings until either side closes.
	<-conn.CloseRead(r.Context()).Done()
}

// VerifyRequest returns the peer whose key signed r at a time near now, in
// the request signing form peers use for the configured service. It reads at
// most 4096 bytes of the body.
func VerifyRequest(r *http.Request, now time.Time, signingEnv string) (peer.ID, error) {
	// Read the small body these routes take, then verify the signature over it.
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		return "", err
	}
	return reqsig.Verify(r, body, signingEnv, now)
}
