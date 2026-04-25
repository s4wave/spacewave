---
name: alpha-debug
description: Use the alpha-debug CLI to inspect and interact with the running page.
---

# Alpha Debug

Use the `alpha-debug` CLI tool to inspect the running Spacewave Alpha page from the terminal.

Quick reference:

```bash
# Get page info
go run ./cmd/alpha-debug/ info

# Evaluate JS expression (auto-returns single expressions)
go run ./cmd/alpha-debug/ eval "document.title"

# Evaluate from file (avoids shell quoting issues)
go run ./cmd/alpha-debug/ eval --file .tmp/script.js

# Evaluate TypeScript from file (bundles with the app aliases first)
go run ./cmd/alpha-debug/ eval --file .tmp/script.ts

# Show exact visual line breaks
go run ./cmd/alpha-debug/ linebreaks "h1"

# Detect typographic orphans
go run ./cmd/alpha-debug/ orphans "p"

# Dump geometry and typography
go run ./cmd/alpha-debug/ measure ".card"

# Check grid row height consistency
go run ./cmd/alpha-debug/ grid-check ".grid"

# Preview text rendering (inject, measure, restore)
go run ./cmd/alpha-debug/ preview-text "h1" "New heading"

# Watch: re-evaluate a JS file on interval
go run ./cmd/alpha-debug/ watch --file .tmp/script.js
```

## Notes

- Prefer `eval --file .tmp/script.js` or `eval --file .tmp/script.ts` for
  anything more than a short expression. Inline shell quoting gets fragile fast.
- Plain expression eval auto-returns the expression value. Multi-line JavaScript
  should use an explicit `return` when a result is needed.
- TypeScript eval files are bundled through Vite, so app aliases such as
  `@s4wave/...` are available.
- The page-side eval result is serialized with `JSON.stringify`. Convert
  `bigint` values to strings in debug scripts before exporting or returning.
- Use `window.__s4wave_debug.root` to access the SDK root from page context.
  When using resource handles in TypeScript eval scripts, dispose only actual
  resource instances. For example, `mountSessionByIdx` returns a wrapper object;
  use `const mounted = await root.mountSessionByIdx(...)` and then
  `using session = mounted.session`.
- If TypeScript eval reports a module MIME/type error or fails to fetch a
  dynamically imported module, check that the debug bridge is serving
  `/p/spacewave-debug/eval/*.js` and the emitted chunks in
  `.bldr/debug/eval/out/`.
