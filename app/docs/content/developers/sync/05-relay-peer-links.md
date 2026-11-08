---
title: Relay Peer Links Through TURN
section: sync
order: 5
summary: Hand each signed-in session short-lived TURN credentials so its WebRTC links connect when no direct path exists.
---

Two peers behind strict NATs or firewalls can't reach each other directly, so
their WebRTC link needs a TURN relay. A relay provider's API token mints
credentials for anyone who holds it, so only your server holds it. The server
issues each signed-in session a credential good for a few hours, and the
session's client uses it for new links.

## Issue credentials on your server

`relay_cloudflare.Issuer` asks Cloudflare's TURN service for six-hour
credentials. `webrtc_relay.NewHandler` serves them to the sessions your `auth`
function accepts:

```go
issuer := &relay_cloudflare.Issuer{
	KeyID:    os.Getenv("TURN_KEY_ID"),
	APIToken: os.Getenv("TURN_API_TOKEN"),
}
mux.Handle("/api/relay", webrtc_relay.NewHandler(issuer, func(r *http.Request) (peer.ID, bool) {
	return sessionPeer(r) // your sign-in check
}))
```

The route answers GET only, refuses requests `auth` rejects, and never lets a
response be cached. An unconfigured issuer or a failed Cloudflare call answers
503 and says nothing about why. Another provider implements
`webrtc_relay.Issuer` the same way.

## Use them on the client

Add a `webrtc_relay.Controller` for the session's peer to its bus. It answers
the transport's `LookupICEProvider` for that peer only, so another session or
plugin on the same bus can't draw on this one's account:

```go
fetch := webrtc_relay.NewHTTPFetch(sessionClient, "https://app.example.com/api/relay")
rel, err := b.AddController(ctx, webrtc_relay.NewController(sessionPeer, fetch, baseConf), nil)
```

`sessionClient` is an `http.Client` that carries the session's sign-in, such as
a cookie jar. `baseConf` is the `WebRtcConfig` the relays are added to: keep
your STUN servers there, and set `IceTransportPolicy` to `RELAY` to force every
link through the relay.

The controller fetches for each new link. It keeps only the `turn:` and
`turns:` URLs that carry a username and credential, fetches again if the
credential expires within a minute, and refuses to start a link if the second
one does too. An established link keeps the credential it started with.

## What stays secret

The API token stays on your server. The client holds each issued credential in
memory for one connection attempt; it never writes it to disk, logs it, or
shows it in directive debug values. Without the controller, the transport uses
its configured ICE servers as before.
