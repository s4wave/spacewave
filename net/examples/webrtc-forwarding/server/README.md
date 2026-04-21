# Signaling Server

This is a basic implementation of a WebRTC signaling server.

It listens on port :2253 for incoming WebSocket connections.

Peers can then signal other peers via the signaling service.

```
NAME:
   webrtc-signaling-server - Hosts a WebSocket server and a signaling service

USAGE:
   webrtc-signaling-server [global options] 

GLOBAL OPTIONS:
   --listen value  address to listen on (default: ":2253") [$LISTEN]
   --http value    http path to listen on (default: "/bifrost-ws") [$HTTP_PATH]
```
