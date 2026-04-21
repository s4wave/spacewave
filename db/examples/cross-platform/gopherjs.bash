#!/bin/bash
set -eo pipefail

echo "Browse to http://localhost:8080/github.com/s4wave/spacewave/db/examples/cross-platform"
echo "Open the console and view the logs."
gopherjs serve -v
