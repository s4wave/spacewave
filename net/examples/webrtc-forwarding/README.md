# HTTP over WebRTC Example

Install Bifrost:

```
go install -v github.com/s4wave/spacewave/net/cmd/bifrost
```

Start the destination service that we will proxy connections to:

```
python3 -m http.server 8080
```

Start the signaling server:

```
cd ./server
go run -v ./
```

Start the first peer, which forwards incoming streams to localhost:8080:

```
bifrost daemon --node-priv ../priv/node-1.pem -c node-1.yaml
```

Start the second peer, which listens on :8084 and forwards incoming traffic to
the other peer via. Bifrost WebRTC:

```
bifrost daemon --node-priv ../priv/node-2.pem -c node-2.yaml
```

Access the forwarded HTTP service via the proxy:

```
curl localhost:8084
```

Or browse to http://localhost:8084 in a web browser.

When opening a connection to the second node at port 8084, Bifrost will open a
WebRTC Link with the other peer on-demand using the demo signaling server and
STUN servers. It will then proxy the traffic to the destination service. Bifrost
will close the Link after a short inactivity period.
