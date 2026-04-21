# Mesh Chat Demo

A peer-to-peer chat demonstrating Bifrost's transport abstraction.

## What This Shows

This demo shows how Bifrost enables direct communication between peers across any transport (UDP, WebSocket, WebRTC) with the same code. Messages route through the mesh network.

## Running the Demo

### Terminal 1 - Start first node
```bash
go run main.go --listen :5000
```

Note: All flags can also be set via environment variables (e.g., `LISTEN=:5000`).

Note the Peer ID that is printed.

### Terminal 2 - Connect to first node
```bash
go run main.go --listen :5001 --dial <PEER_ID_FROM_TERMINAL_1>@127.0.0.1:5000
```

### Start Chatting
Type messages in either terminal and press Enter. Messages are sent directly peer-to-peer over encrypted QUIC streams.

## Key Features

- **Transport Agnostic**: Same code works over UDP, WebSocket, or WebRTC
- **Encrypted**: All traffic uses TLS 1.3 via QUIC
- **P2P**: Direct peer-to-peer, no servers required

## Architecture

```
┌─────────────┐         QUIC/UDP          ┌─────────────┐
│   Peer A    │◄─────────────────────────►│   Peer B    │
│  :5000      │    Encrypted Stream       │  :5001      │
└─────────────┘                           └─────────────┘
```

## Testing

Run the end-to-end tests:

```bash
go test -tags test_examples .
```

Tests verify:
- Multi-hop routing (A → B → C)
- Broadcast to multiple peers
- Message delivery guarantees

## Transport Options

The same chat code works unchanged over:
- **UDP/QUIC** - Low latency, NAT traversal via UDP
- **WebSocket** - Works through corporate firewalls
- **WebRTC** - Browser-to-browser without servers

Change the transport configuration without code changes.
