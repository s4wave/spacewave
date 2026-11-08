// Package relay_cloudflare issues Cloudflare TURN credentials.
//
// The Issuer runs on the app server, which holds the TURN key's API token. It
// implements webrtc_relay.Issuer, so NewHandler serves its credentials to
// signed-in sessions.
package relay_cloudflare

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/fastjson"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	webrtc_relay "github.com/s4wave/spacewave/net/transport/webrtc/relay"
)

// Endpoint is the Cloudflare API that issues TURN credentials.
const Endpoint = "https://rtc.live.cloudflare.com/v1/turn/keys/"

// Lifetime is how long issued credentials stay valid.
const Lifetime = 6 * time.Hour

// maxResponseSize bounds the Cloudflare response.
const maxResponseSize = 64 * 1024

// Issuer issues credentials for one Cloudflare TURN key.
type Issuer struct {
	// KeyID is the TURN key's identifier.
	KeyID string
	// APIToken is the TURN key's API token. It never leaves the server.
	APIToken string
	// Client sends the request; nil uses http.DefaultClient.
	Client *http.Client
	// Endpoint overrides the API base URL; empty uses Endpoint.
	Endpoint string
	// Now overrides the clock; nil uses time.Now.
	Now func() time.Time
}

// Issue asks Cloudflare for credentials valid for Lifetime.
//
// An unconfigured issuer and any upstream failure return
// webrtc_relay.ErrUnavailable, without the upstream reply.
func (i *Issuer) Issue(ctx context.Context, _ peer.ID) (*webrtc_relay.Credentials, error) {
	// Without a key nothing can be issued.
	if i == nil || i.KeyID == "" || i.APIToken == "" {
		return nil, webrtc_relay.ErrUnavailable
	}
	client, base, now := i.Client, i.Endpoint, i.Now
	if client == nil {
		client = http.DefaultClient
	}
	if base == "" {
		base = Endpoint
	}
	if now == nil {
		now = time.Now
	}

	// Ask for one credential; the expiry counts from before the request.
	issuedAt := now()
	body := `{"ttl":` + strconv.Itoa(int(Lifetime/time.Second)) + `}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+url.PathEscape(i.KeyID)+"/credentials/generate-ice-servers", strings.NewReader(body))
	if err != nil {
		return nil, webrtc_relay.ErrUnavailable
	}

	// Authenticate with the token; only this request carries it.
	req.Header.Set("Authorization", "Bearer "+i.APIToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, webrtc_relay.ErrUnavailable
	}
	defer resp.Body.Close()

	// Cloudflare answers 201 Created with a small JSON body.
	if resp.StatusCode/100 != 2 {
		return nil, webrtc_relay.ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil || len(data) > maxResponseSize {
		return nil, webrtc_relay.ErrUnavailable
	}

	// Keep the servers as issued; the client keeps only the credentialed relays.
	value, err := fastjson.ParseBytes(data)
	if err != nil {
		return nil, webrtc_relay.ErrUnavailable
	}
	creds := &webrtc_relay.Credentials{ExpiresAt: timestamppb.New(issuedAt.Add(Lifetime))}
	for _, srv := range value.GetArray("iceServers") {
		conf := &webrtc.IceServerConfig{Username: string(srv.GetStringBytes("username"))}
		for _, u := range srv.GetArray("urls") {
			conf.Urls = append(conf.Urls, string(u.GetStringBytes()))
		}
		if pass := string(srv.GetStringBytes("credential")); pass != "" {
			conf.Credential = &webrtc.IceServerConfig_Password{Password: pass}
		}
		creds.IceServers = append(creds.IceServers, conf)
	}
	if len(creds.RelayServers()) == 0 {
		return nil, webrtc_relay.ErrUnavailable
	}
	return creds, nil
}

// _ is a type assertion
var _ webrtc_relay.Issuer = (*Issuer)(nil)
