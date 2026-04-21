#!/bin/bash
set -eo pipefail

# Build the WASM first
echo "Building WASM..."
bash build.bash

echo ""
echo "========================================"
echo "Starting HTTP server on port 8000"
echo "========================================"
echo "Open your browser to: http://localhost:8000"
echo ""
echo "Before using the demo:"
echo "1. Start the signaling server in ../server/"
echo "2. Start the backend node in ../backend/"
echo "3. Start a local HTTP service (e.g., python3 -m http.server 8080 in another directory)"
echo "4. Configure peer IDs in the web interface"
echo ""

# Serve on port 8000 with CORS support
# Using Python's built-in HTTP server if available, otherwise suggest npx serve
if command -v python3 &> /dev/null; then
    python3 -m http.server 8000
elif command -v python &> /dev/null; then
    python -m http.server 8000
elif command -v npx &> /dev/null; then
    npx serve --cors -p 8000
else
    echo "ERROR: No HTTP server available. Please install Python or Node.js"
    exit 1
fi
