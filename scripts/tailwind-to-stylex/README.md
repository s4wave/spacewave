# Tailwind to StyleX

Use the current application theme as the compiler oracle:

```sh
bun run stylex:migrate --report .bldr/stylex-inventory.jsonl app web plugin
bun run stylex:migrate --write --report .bldr/stylex-web.jsonl web/ui
```

The default is a dry run. Each file is parsed, translated and checked for
idempotence before it is written. JSONL output records converted sites and
remaining expressions; its final row gives counts and CSS property registrations
needed by the converted utilities.

Supported expressions include literal strings, immutable local class constants,
imported `cn` aliases, arrays, conditional branches and conditional class objects.
Static local `cva` recipes retain defaults, null choices, boolean variants and
compound variants through typed StyleX composition. Exported recipes, recipes
with other runtime consumers, and modules with an unsupported attribute keep
their existing recipe for a coordinated hand pass.

Tailwind supplies declarations, canonical class order, opacity expressions,
pseudo-classes and media/container conditions. Known theme variables become
named imports from `tokens.stylex.ts`. Compiler bindings avoid existing names.
Unused consumed class constants and single-purpose imports are removed.

An unsupported attribute remains whole, with a `TODO(stylex)` reason. Descendant,
group and peer selectors need styles on the affected child or explicit markers.
Global classes and computed class helpers also need a hand pass. Shared-component
API conversion and replacing forwarded `className` with `sx` are coordinated
with their callers in that pass.

Run `test:stylex-migrate` for the real compiler and browser comparison. It builds
original and converted components with Vite and compares resolved properties,
geometry, accessibility and rendered pixels in Chromium and WebKit at three
sizes. The historical component inputs stay fixed when their production files
convert. `STYLEX_PILOT_SOURCE=1` compares those inputs against the handwritten
production pilots. `STYLEX_MIGRATION_ARTIFACTS=1` retains passing captures;
failed runs always retain theirs.

Use `--component-api` for the coordinated shared-component pass. It changes
owned `className` inputs to typed `sx` inputs and composes the caller's styles
last. A file with remaining unsupported attributes keeps its old input contract
for the hand pass. `--shared-sx` rewrites styling inputs on components imported
from `web/ui`; use it on callers after those shared APIs have been converted.
Other components retain their own input contract until their area converts.

Class strings passed through `cn` use the actual `cnfast` normalizer before
utility extraction. Plain JSX class strings retain Tailwind's canonical CSS
order. This distinction preserves cases such as `cn('p-2 p-1')`, where the
helper removes `p-2` before the browser sees it.
