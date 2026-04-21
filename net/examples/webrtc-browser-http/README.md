# HTTP over WebRTC from Browser

This example demonstrates calling an HTTP service via a WebRTC bridge from a web browser.

The browser client establishes a WebRTC connection to a backend peer, which forwards HTTP requests to a local HTTP service. This allows the browser to access services that are not directly accessible via traditional HTTP, using encrypted peer-to-peer WebRTC connections.

## Architecture

1. **Browser Client** - WebAssembly application running in the browser
2. **Signaling Server** - Facilitates WebRTC peer discovery and connection setup
3. **Backend Node** - Bifrost daemon that forwards requests to the HTTP service
4. **HTTP Service** - The target service (e.g., a simple Python HTTP server)

```
Browser (WebRTC) <-> Signaling Server <-> Backend Node <-> HTTP Service
```

## Prerequisites

- Go 1.21 or higher
- Python 3 (for serving the browser client and HTTP service)
- Modern web browser with WebAssembly support

## Running the Example

### Step 1: Start the HTTP Service

Start a simple HTTP server that we will proxy connections to:

```bash
python3 -m http.server 8080
```

Keep this running in a separate terminal.

### Step 2: Start the Signaling Server

The signaling server facilitates WebRTC connection setup:

```bash
cd server
go run main.go
```

The signaling server will start and print:

```
INFO starting signaling server with peer id: 12D3KooWNyn6cNNxHnLc5Nw8b7XkVaAWKB9vbfe921LuysEoY1Cz
```

Keep this running in a separate terminal.

### Step 3: Start the Backend Node

The backend node forwards incoming streams to localhost:8080:

```bash
cd backend
go run main.go
```

The backend node will start and print:

```
INFO backend node starting with peer id: 12D3KooWADAL8c4BbWYWxjXZGNMRx4HJAJCtnu6wk6wmux7scvk7
```

Keep this running in a separate terminal.

### Step 4: Build and Serve the Browser Client

Build the WebAssembly client and start the web server:

```bash
cd browser
./build.bash
./serve.bash
```

This will serve the browser client at http://localhost:8000

### Step 5: Open in Browser

**IMPORTANT:** Disable any VPN connection before testing. VPNs can interfere with WebRTC ICE candidate exchange and prevent peer connections from establishing.

1. Browse to http://localhost:8000 in your web browser
2. Open the developer console (F12) to see debug logs
3. The peer IDs should be pre-filled from the configuration
4. Click "Connect WebRTC" button to establish connection
5. Wait for "WebRTC connection ready!" message in the console (this may take a few seconds)
6. Enter a URL path (e.g., `/`) and click "Fetch via WebRTC"
7. The response from the HTTP service will appear in the console

## Configuration

The example uses these peer IDs (generated from keys in `../priv/`):

- **Signaling Server**: `12D3KooWNyn6cNNxHnLc5Nw8b7XkVaAWKB9vbfe921LuysEoY1Cz` (from `../priv/node-3.pem`)
- **Backend Node**: `12D3KooWADAL8c4BbWYWxjXZGNMRx4HJAJCtnu6wk6wmux7scvk7` (from `../priv/backend-node.pem`)
- **Protocol ID**: `webrtc-browser-http/v1`

These values are configured in:

- `browser/index.html` - Browser client configuration
- `backend/main.go` - Backend node configuration

## Troubleshooting

### WebRTC Connection Fails or Times Out

**Disable VPN:** VPNs and network security software can block WebRTC connections. Disable your VPN before running the example.

**Check Firewall:** Ensure your firewall allows WebRTC traffic. The example uses STUN servers on ports 19302 and 3478.

**Check Logs:** Open the browser developer console (F12) to see detailed connection logs. Look for:

- "WASM functions ready!" - confirms the WASM module loaded
- "Loaded signaling server ID from HTML" - confirms peer IDs are set
- "WebSocket transport established" - confirms signaling connection
- "Link established with backend peer!" - confirms WebRTC connection succeeded

### Bus Not Initialized Error

If you see "Bus not initialized. Call connectWebRTC() first", make sure you clicked the "Connect WebRTC" button and waited for the "WebRTC connection ready!" message before trying to fetch.

## Notes

- **VPN Warning:** Disable VPNs before testing - they interfere with WebRTC ICE candidate exchange
- The example uses Google's public STUN servers for NAT traversal
- In production, you would use your own TURN/STUN infrastructure
- The signaling server must be accessible to both browser and backend
- Private keys are stored in `../priv/` directory (generated on first run)
- The browser client serves on port 8000, signaling on port 2253, target HTTP on port 8080
