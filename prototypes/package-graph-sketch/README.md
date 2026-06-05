# Spacewave GoScript Package Graph Sketch

This prototype answers one question first:

> Which browser packages are still compiled from Go, which are hand-written
> TypeScript substitutes in `gs/`, and where are the manual override hotspots?

It emits a readable overview SVG by default plus a JSON sidecar with the full
Go, generated GoScript, manual `gs/` override, and TypeScript package graph.

Default use from the Spacewave repo:

```sh
bun prototypes/package-graph-sketch/graph.ts \
  --generated-root .bldr-dist/build/web/js/wasm/spacewave-core/dist/@goscript \
  --generated-root .bldr-dist/build/web/js/wasm/spacewave-launcher/dist/@goscript \
  --override-root ../goscript/gs \
  --out .tmp/package-graph-sketch.svg
```

The overview SVG keeps a fixed center divider:

- Go packages and generated GoScript output packages are on the left.
- Manual GoScript `gs/` override packages and ordinary TypeScript packages are
  on the right.
- The primary view aggregates by package family and lists the override packages
  to inspect first, instead of drawing every import edge.

Use `--mode full` when you want the dense package/edge graph:

```sh
bun prototypes/package-graph-sketch/graph.ts \
  --mode full \
  --generated-root .bldr-dist/build/web/js/wasm/spacewave-core/dist/@goscript \
  --override-root ../goscript/gs \
  --out .tmp/package-graph-sketch-full.svg
```

Useful options:

- `--go-package <pattern>`: repeatable Go package pattern. Defaults to
  Spacewave's main repo surfaces.
- `--go-tags <tags>`: defaults to `skip_e2e,purego,goscript`.
- `--mode <overview|full>`: defaults to `overview`.
- `--ts-root <dir>`: repeatable TypeScript source root. Defaults to the roots
  in the repo `tsconfig.json` include set.
- `--generated-root <dir>`: repeatable root containing an `@goscript` tree.
- `--override-root <dir>`: repeatable GoScript manual override root such as
  `../goscript/gs`.
- `--max-nodes <n>`: cap the rendered graph while keeping the JSON sidecar
  complete enough for inspection.
