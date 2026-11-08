package publisher

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// SelectorMaxAge is the seconds a fetched selector may be cached. Launchers
// polling an endpoint see a new release within a minute; connected clients
// receive it at once through Forward.
const SelectorMaxAge = 60

// Selector holds a Deployment's newest signed DistConfig. It serves the
// selector to launchers that fetch it and forwards each newer one to
// connected clients.
type Selector struct {
	// bcast guards below fields and wakes Forward on a new selector.
	bcast broadcast.Broadcast
	// rev is the control revision of signed.
	rev uint64
	// signed is the signed packed message launchers verify.
	signed string
}

// Set selects signed, the DistConfig at control revision rev, when rev is
// above the current revision. Returns false and keeps the current selector
// otherwise, so a late or repeated release never regresses what clients see.
func (s *Selector) Set(rev uint64, signed string) bool {
	// Swap in a higher revision and wake the forwarders.
	set := false
	s.bcast.HoldLock(func(wake func(), _ func() <-chan struct{}) {
		if rev <= s.rev || signed == "" {
			return
		}
		s.rev, s.signed = rev, signed
		set = true
		wake()
	})
	return set
}

// SetReleased selects the selector a Release or Reissue signed.
func (s *Selector) SetReleased(r *Released) bool {
	return s.Set(r.DistConfig.GetRev(), r.Signed)
}

// Get returns the current control revision and signed selector, or zero and
// empty before the first Set.
func (s *Selector) Get() (uint64, string) {
	var rev uint64
	var signed string
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rev, signed = s.rev, s.signed
	})
	return rev, signed
}

// ServeHTTP serves the signed selector a launcher endpoint fetches, cached
// for SelectorMaxAge seconds. It responds 404 before the first Set.
func (s *Selector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Serve reads only.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Respond not found until a release sets the selector.
	_, signed := s.Get()
	if signed == "" {
		http.NotFound(w, r)
		return
	}

	// Send the signed packed message with a one-minute cache.
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "public, max-age="+strconv.Itoa(SelectorMaxAge))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = strings.NewReader(signed).WriteTo(w)
}

// Forward sends the current selector, then each newer one, to one connected
// client until ctx ends or send fails. A server runs it per client
// connection; the client hands each message to its launcher with
// PushToLauncher.
func (s *Selector) Forward(ctx context.Context, send func(ctx context.Context, signed string) error) error {
	var sent uint64
	for {
		// Read the current selector and the wake for the next one.
		var rev uint64
		var signed string
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			rev, signed, waitCh = s.rev, s.signed, getWaitCh()
		})

		// Send it unless the client already has it.
		if rev > sent {
			if err := send(ctx, signed); err != nil {
				return err
			}
			sent = rev
		}

		// Wait for a newer selector or the end of the connection.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// PushToLauncher returns a send function for Forward that pushes each signed
// selector to the launcher on the client's bus b. The launcher verifies the
// selector as it would a fetched one and adopts it only when its revision is
// higher, so a stale or badly signed selector changes nothing; a bad
// signature or channel returns its error.
func PushToLauncher(b bus.Bus) func(ctx context.Context, signed string) error {
	return func(ctx context.Context, signed string) error {
		_, err := spacewave_launcher.ExPushDistConfigMsg(ctx, b, signed)
		return err
	}
}
