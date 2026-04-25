# Spacewave

Spacewave is a local-first application workspace built with Go, TypeScript,
React, and Bldr.

## Setup

Install dependencies:

```bash
bun install
```

## Run

Desktop:

```bash
bun run start:desktop
```

Browser:

```bash
bun run start:web
```

WASM:

```bash
bun run start:web:wasm
```

## Generate

Regenerate protobuf outputs after changing `.proto` files:

```bash
bun run gen
```

## Test

Run the main test targets:

```bash
bun run test
```

Useful focused targets:

```bash
bun run test:js
bun run test:browser
bun run test:go
```

## Development

Format, lint, and typecheck:

```bash
bun run format
bun run lint
bun run typecheck
```
