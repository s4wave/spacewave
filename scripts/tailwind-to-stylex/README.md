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
compound variants through complete finite StyleX outcomes. Each outcome merges
its whole class set before compilation, preserving base hover and responsive
values beneath variant overrides. Exported recipes, recipes
with other runtime consumers, and modules with an unsupported attribute keep
their existing recipe for a coordinated hand pass.

Tailwind supplies declarations, canonical class order, opacity expressions,
pseudo-classes and media/container conditions. Known theme variables become
named imports from `tokens.stylex.ts`. Compiler bindings avoid existing names.
Unused consumed class constants and single-purpose imports are removed.

An unsupported attribute remains whole, with a `TODO(stylex)` reason. Descendant,
group and peer selectors need styles on the affected child or explicit markers.
Global classes and computed class helpers also need a hand pass. Mixed logical axes and physical sides with responsive or pseudo conditions stay
manual, because StyleX atomic priorities can change their cascade. Shared-component
API conversion and replacing forwarded `className` with `sx` are coordinated
with their callers in that pass. Native roots use `stylexProps` to retain the
variables emitted for dynamic styles beside forwarded native inline styles.
Wrappers delegate `sx` to the component that renders their DOM.

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
for the hand pass. `--shared-sx` reads the current exports of imported `web/ui`
modules and uses `sx` for components whose API has converted. Other components
keep their existing input contract. Use `--shared-callers` to change only those
converted components' call sites, leaving other elements for their area's
conversion.

Class strings passed through `cn` use the actual `cnfast` normalizer before
utility extraction. Plain JSX class strings retain Tailwind's canonical CSS
order. This distinction preserves cases such as `cn('p-2 p-1')`, where the
helper removes `p-2` before the browser sees it.

Animation utilities become explicit longhands through the native CSS parser;
StyleX drops the CSS `animation` shorthand. Font-mono styles also carry the
repository's console rendering properties, which formerly depended on its
utility class. Default font rendering belongs to the reset layer.

Shared icons are exported from `web/images/icons.ts` so controls and caller
icons share one React context across native package boundaries. Button's icon
rules use a named marker in `button-icons.stylex.ts`; port other descendant
rules to their SVG owner or an explicit marker too.

Use `STYLEX_MIGRATION_STATES=buttons-default,buttons-focused,buttons-hovered`
for a focused comparison with the same capture protocol. The default runs every
state; record the selected states with partial-run evidence.
