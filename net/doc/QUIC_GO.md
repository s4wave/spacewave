# quic-go in Bifrost

Bifrost uses [quic-go](https://github.com/quic-go/quic-go) as the underlying
QUIC implementation for its peer-to-peer transport layer. QUIC provides
multiplexed streams over a single connection with built-in encryption via TLS
1.3, making it well-suited for bifrost's networking needs.

## How Bifrost Uses quic-go

Bifrost wraps quic-go to build authenticated P2P links between peers:

- **transport/common/quic**: Core QUIC transport abstraction. Wraps `quic.Conn`
  into bifrost `Link` objects with peer identity verification via mTLS
  (libp2p-tls). Handles dialing, listening, and stream multiplexing.

- **transport/common/pconn**: Packet-conn transport. Runs QUIC over raw UDP
  sockets using `quic.Transport`. Used for direct UDP-based connections.

- **transport/common/conn**: Ordered-conn transport. Runs QUIC over ordered
  byte streams (e.g. TCP) by packetizing them with a custom `PacketConn`
  adapter.

- **transport/websocket**: Runs QUIC over WebSocket connections. WebSocket
  frames are treated as packets for the QUIC layer.

- **transport/webrtc**: Runs QUIC over WebRTC data channels. Enables
  browser-to-browser and browser-to-native P2P connections.

All transports negotiate peer identity using libp2p-tls certificates. The QUIC
connection provides the stream multiplexing and flow control, while the TLS
layer provides mutual authentication and peer identity verification.

### Configuration

QUIC configuration is built via `BuildQuicConfig(opts)` which maps bifrost
`Opts` protobuf fields to `quic.Config`:

- `MaxIdleTimeoutDur`: Connection idle timeout (default: 10s)
- `MaxIncomingStreams`: Maximum concurrent streams (default: 100000)
- `DisableDatagrams`: Disable QUIC datagram support
- `DisableKeepAlive`: Disable keep-alive probes
- `KeepAliveDur`: Custom keep-alive interval
- `DisablePathMtuDiscovery`: Disable PMTU discovery (forced on for
  non-UDP transports like WebSocket, WebRTC, and ordered conns)

Bifrost uses QUIC Version 2 (RFC 9369) exclusively.

## Fork History

Bifrost previously used a fork at `github.com/aperturerobotics/quic-go`. The
fork was created in January 2024 and maintained the following changes on top of
upstream:

1. **Configurable logrus logger**: Added `Logger *logrus.Entry` field to
   `Config` and `Transport`, replacing quic-go's built-in logger (which uses
   Go's `log` package and `QUIC_GO_LOG_LEVEL` env var) with logrus integration.
   This was the primary reason for the fork.

2. **`CloseNoError()` method**: Added a convenience method on `Connection` that
   calls `closeLocal(nil)` and waits for context cancellation. Equivalent to
   `CloseWithError(0, "")`.

3. **Log level reductions**: Demoted ~9 log messages from Info/Error to Debug
   level to reduce noise during normal operation.

4. **Buffer size warning suppression**: Commented out UDP buffer size warnings
   that print on startup when the OS buffer cannot be increased.

5. **Multiplexer removal**: Removed the global connection multiplexer (upstream
   later removed this independently).

6. **Mutex deadlock fix**: Used `TryLock` pattern in `closeServer()` to avoid
   a potential deadlock when closing transports.

### Why the Fork Was Dropped

As of February 2026, the fork was dropped in favor of upstream quic-go v0.59.0:

- The `CloseNoError()` convenience method was replaced with the upstream
  idiomatic `CloseWithError(0, "")` (one call site in bifrost).
- The configurable logger was the only feature that truly required a fork.
  Bifrost decided to accept upstream's default logging behavior (silent by
  default, configurable via `QUIC_GO_LOG_LEVEL` env var) rather than maintain
  a fork indefinitely.
- Buffer size warnings can be suppressed via the
  `QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING=true` env var.
- Log level reductions are less important since upstream defaults to no logging.
- The multiplexer was already removed upstream.
- The deadlock fix was specific to the fork's older architecture and not
  applicable to current upstream.

## Upstream Wishlist

Features that would benefit bifrost if added to upstream quic-go:

1. **Configurable logger interface**: The ability to inject a custom logger
   (e.g. `slog.Logger` or a simple interface) into `Config` or `Transport`
   instead of relying on `QUIC_GO_LOG_LEVEL` + Go's `log` package. Upstream
   has a TODO comment for this in `transport.go`. This was the primary reason
   for the fork and would eliminate the need for any future fork.

2. **Programmatic buffer warning control**: While
   `QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING` exists, a programmatic way to
   suppress or redirect the buffer size warning (e.g. via the logger interface
   above) would be cleaner for library consumers.
