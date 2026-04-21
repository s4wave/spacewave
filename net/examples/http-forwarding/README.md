# HTTP Forwarding Example

Install Bifrost:

```
go install -v github.com/s4wave/spacewave/net/cmd/bifrost
```

Start the destination service that we will proxy connections to:

```
python3 -m http.server 8080
```

Start the first peer, which forwards incoming streams to localhost:8080:

```
bifrost daemon --node-priv ../priv/node-1.pem -c node-1.yaml
```

Start the second peer, which listens on :8084 and forwards incoming traffic to
the other peer via. Bifrost:

```
bifrost daemon --node-priv ../priv/node-2.pem -c node-2.yaml
```

Access the forwarded HTTP service via the proxy:

```
curl localhost:8084
```

Or browse to http://localhost:8084 in a web browser.

When opening a connection to the second node at port 8084, Bifrost will open a
Quic-based Link with the other peer on-demand and proxy the traffic to the
destination service over a stream. When the proxy becomes idle, Bifrost will
close the Link after a short inactivity period.

## Configuration

The first node is configured to listen with Quic-over-UDP and forward incoming
streams to the `my/http/forwarding` protocol ID to the target multiaddress
`/ip4/127.0.0.1/tcp/8080`:

```yaml
udp:
  id: bifrost/udp
  config:
    listenAddr: :50042

forwarding:
  id: bifrost/stream/forwarding
  config:
    protocolId: my/http/forwarding
    targetMultiaddr: "/ip4/127.0.0.1/tcp/8080"
```

The second node is configured to dial the other node via Quic-over-UDP and
forward incoming connections to `/ip4/127.0.0.1/tcp/8084` to the other node at
the protocol ID `my/http/forwarding`:

```yaml
udp:
  id: bifrost/udp
  config:
    dialers:
      12D3KooWC9dBAEoTHbEXq2aaTeFit7QVdvPcb6Yf76oGQZ6dGf8N:
        address: 127.0.0.1:50042
    listenAddr: :50043

listening:
  id: bifrost/stream/listening
  config:
    remotePeerId: 12D3KooWC9dBAEoTHbEXq2aaTeFit7QVdvPcb6Yf76oGQZ6dGf8N
    protocolId: my/http/forwarding
    listenMultiaddr: "/ip4/127.0.0.1/tcp/8084"
```

### Changing the transport

You can edit this yaml file and add unlimited additional controllers, for
example, a Websocket server:

```yaml
websocket:
  id: bifrost/websocket
  listenAddr: :50050
```

And a websocket client:

```yaml
websocket:
  id: bifrost/websocket
  dialers:
    12D3KooWC9dBAEoTHbEXq2aaTeFit7QVdvPcb6Yf76oGQZ6dGf8N:
      address: ws://127.0.0.1:50050/bifrost
```

...then the traffic will be forwarded via WebSocket instead of UDP.
