#!/bin/bash
# Bifrost Pipe Demo Script
# This script demonstrates the bifrost pipe command

set -e

echo "=== Bifrost Pipe Demo ==="
echo ""

# Check if bifrost is available
if ! command -v bifrost &> /dev/null; then
    echo "Error: bifrost command not found. Please run 'go install ./cmd/bifrost' first."
    exit 1
fi

# Clean up any existing processes
cleanup() {
    echo ""
    echo "Cleaning up..."
    kill $SERVER_PID 2>/dev/null || true
    rm -f /tmp/pipe_demo_server.log /tmp/pipe_demo_received.txt
}
trap cleanup EXIT

# Start the server
echo "Starting server on :5555..."
bifrost pipe -l :5555 > /tmp/pipe_demo_received.txt 2>/tmp/pipe_demo_server.log &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Get the peer ID from server log
PEER_ID=$(grep "Peer ID:" /tmp/pipe_demo_server.log | awk '{print $3}')

if [ -z "$PEER_ID" ]; then
    echo "Error: Could not get server peer ID"
    cat /tmp/pipe_demo_server.log
    exit 1
fi

echo "Server started with Peer ID: $PEER_ID"
echo ""

# Send a message through the pipe
echo "Sending message through pipe..."
echo "Hello from bifrost pipe demo!" | bifrost pipe -q -c "$PEER_ID@127.0.0.1:5555"

# Wait for data to be received
sleep 1

# Show what was received
echo ""
echo "=== Message received by server ==="
cat /tmp/pipe_demo_received.txt
echo "=== End of message ==="
echo ""

echo "=== Demo Complete ==="
echo ""
echo "You can now run your own experiments:"
echo ""
echo "  Server: bifrost pipe -l :5112"
echo "  Client: bifrost pipe -c <PEER_ID>@127.0.0.1:5112"
echo ""
