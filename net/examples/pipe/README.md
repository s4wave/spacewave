# Bifrost Pipe Example

This example demonstrates the `bifrost pipe` command, which provides netcat/socat-like functionality for streaming stdin/stdout over bifrost's QUIC-over-UDP transport.

## Overview

The `pipe` command creates an in-process bifrost daemon and establishes a bidirectional stream between two endpoints. No separate daemon process is required.

## Usage

### Server Mode

Start a server that listens for incoming connections:

```bash
bifrost pipe -l :5112
```

This will output the server's peer ID:
```
Peer ID: 12D3KooW...
Listening on :5112
```

### Client Mode

Connect to a server using the peer ID and address:

```bash
bifrost pipe -c 12D3KooW...@hostname:5112
```

## Examples

### Simple Text Piping

**Terminal 1 (Server):**
```bash
bifrost pipe -l :5112
# Prints peer ID, then waits for connection
# Any data received from client will be printed to stdout
```

**Terminal 2 (Client):**
```bash
echo "Hello, Bifrost!" | bifrost pipe -c <PEER_ID>@127.0.0.1:5112
```

### Audio Streaming

Stream audio from a Linux server to a macOS client:

**Server (Linux with PipeWire):**
```bash
pw-record --format=s16 --rate=48000 --channels=2 - | bifrost pipe -l :5112
```

**Client (macOS):**
```bash
bifrost pipe -c <PEER_ID>@server:5112 | ffplay -nodisp -f s16le -ar 48000 -ac 2 -i -
```

### File Transfer

**Server:**
```bash
cat largefile.bin | bifrost pipe -l :5112
```

**Client:**
```bash
bifrost pipe -c <PEER_ID>@server:5112 > received_file.bin
```

### Persistent Identity

Use a persistent private key for a stable peer ID:

```bash
# Server with persistent key
bifrost pipe -l :5112 -k ./server.pem

# The peer ID will be the same across restarts
```

## Command Options

| Flag | Short | Description |
|------|-------|-------------|
| `--listen` | `-l` | Listen address for server mode (e.g., `:5112`) |
| `--connect` | `-c` | Connect address for client mode (`peer-id@host:port`) |
| `--protocol-id` | `-p` | Stream protocol ID (default: `/pipe/stream`) |
| `--key` | `-k` | Path to private key file (auto-generated if doesn't exist) |
| `--quiet` | `-q` | Suppress status messages to stderr |

## Demo Script

Run the included demo script to see the pipe command in action:

```bash
./demo.sh
```

## How It Works

1. The command creates an in-process bifrost daemon with a UDP transport
2. In server mode, it registers a stream handler for the protocol ID and waits for connections
3. In client mode, it dials the remote peer using the QUIC-over-UDP transport
4. Once connected, stdin is piped to the stream and the stream is piped to stdout
5. The connection uses the "hold-open" controller to prevent timeouts during streaming
