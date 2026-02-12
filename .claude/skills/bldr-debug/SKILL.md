---
name: bldr-debug
description: Use the bldr debug CLI to inspect and interact with the running saucer webview.
---

# Bldr Debug

Use the `bldr debug` CLI tool to evaluate JavaScript in the running saucer webview from the terminal. Read `.opencode/knowledge/saucer.md` for full documentation on the debug bridge architecture and troubleshooting.

Quick reference:

```bash
# Evaluate JS expression
go run ./cmd/bldr/ debug eval "document.title"

# Evaluate from file (avoids shell quoting issues)
go run ./cmd/bldr/ debug eval --file .tmp/script.js
```

## How it works

1. `bldr debug eval` connects to a Unix socket at `.bldr/saucer-debug.sock`
2. The debug bridge in Go forwards the request via yamux to the C++ saucer process
3. C++ executes the JS in the webview and returns the result
4. The socket path can be overridden with the `BLDR_DEBUG_SOCK` env var

## Tips

- Use `--file` (`-f`) for multi-line or complex JS to avoid shell quoting issues
- Write scripts to `.tmp/` (gitignored) for reusable debug snippets
- Single expressions are auto-returned; wrap multi-statement code in an IIFE if needed
