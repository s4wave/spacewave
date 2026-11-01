# Style parity

Install Playwright's Chromium and WebKit once on the capture host:

```sh
bun run style-parity:install-browsers
```

These browser versions belong to the Node Playwright package, independently of
playwright-go's browser installation. Node 24 or later runs the collector.

Use the signed local CDN fixture to capture the branch's own app and plugins:

```sh
STYLE_PARITY_OUTPUT=.bldr/parity-baseline STYLE_PARITY_REPEAT=1 bun run style-parity:release
```

This builds and serves the release and prerender outputs through the existing
release test harness. It keeps outside requests on the fixture's rejecting proxy.
`STYLE_PARITY_REPEAT=1` captures twice on the same origin and compares the results.
Set `STYLE_PARITY_MANIFEST` to choose another manifest. A plain release directory
contains the bootstrap; its plugins may come from another distribution revision.

For an already running, isolated release fixture, capture and compare the same manifest
against another revision. Node 24 or later runs Playwright; Bun compiles the
tool. Browser contexts use disposable storage.

```sh
bun run style-parity capture --base http://127.0.0.1:5595 --revision SERVED_COMMIT \
  --manifest scripts/style-parity/public.manifest.ts --out .bldr/parity-before
bun run style-parity capture --base http://127.0.0.1:5595 --revision SERVED_COMMIT \
  --manifest scripts/style-parity/public.manifest.ts --out .bldr/parity-after
bun run style-parity compare --before .bldr/parity-before \
  --after .bldr/parity-after --out .bldr/parity-differences.jsonl
```

Pass the revision that built the served application, even when the collector
runs in another checkout. `--source-status` records the build checkout's Git
status; its default is `unspecified`. The signed fixture supplies both values.
A dirty checkout requires recording its changes alongside the capture.

Use a new output directory for each capture. The tool saves screenshots,
resolved CSS properties, pseudo-element styles, geometry and accessible
content for every state in Chromium and WebKit at three sizes. It excludes
custom property declarations because different compilers use different internal
variables; their effects remain visible in the resolved CSS properties.

Manifests run directly in Node 24. Runtime imports between source manifests
name their `.ts` files, since Node does not remap `.js` imports to TypeScript.
A manifest exports `states`. Each state has an `id`, `path`, and `ready`, plus
an optional `setup(page)` function that opens the required state. `ready` is
either a selector for the final state or an async function that waits for the
application's own readiness. A heading appearing during loading is insufficient.
Each state starts in a fresh context, so setup must establish its own disposable
fixture. The starter manifest covers three public pages; add session viewers
and overlays before treating it as full application coverage.

The capture fixes wall-clock dates, waits for fonts and image decoding, finishes
finite transitions and cancels repeating effects to sample their underlying
static styles. SVG timelines stop at zero too. The screenshot waits for the
static paint and hides the caret. Its cleanup may restart CSS animations, so
the collector reestablishes that static sample before reading computed styles.
PNGs are retained; comparison hashes use decoded pixels rather than PNG
compression or metadata. This records static styles and geometry. Animation
timing and interaction quality need separate checks.

An explicit Chromium `ERR_NETWORK_CHANGED` retries once in a fresh context.
The first attempt's error, screenshot and accessibility tree are retained.
Other failures remain terminal for that state.

The final `capture.json` appears only after every state succeeds. Failed states
save an error record and a diagnostic screenshot and accessibility tree when
the page can still supply them. The tool continues with the remaining states,
then fails the command. Comparison requires the same manifest and capture matrix
and exits with status 1 when any structured value or significant pixel differs.
A lone image difference of at most one 8-bit channel level is recorded in the
separate `.raster.jsonl` report when all computed properties, geometry and
accessibility match exactly. Chromium can vary by that rounding level between
otherwise identical text paints. Any structured change retains its screenshot
difference, including a real one-level CSS color change. Raw captures and
screenshots are retained. The JSONL report streams one
pair of screens at a time.

Run `bun run test:style-parity` for the repeatability and single-property check,
and `bun run check:style-parity` and `bun run lint:style-parity` for source checks.
Set `STYLE_PARITY_TEST_ARTIFACTS` to a directory to retain the test's captures.

Selection snapshots include the painted properties declared by application CSS,
including declarations inherited through highlight ancestors. Ordinary element
styles already cover the default inherited text color. Chromium can return a
stale transition color for an unstyled highlight, so its unused pseudo defaults
are not a second source of that color. Explicit selection-color changes remain
part of the comparison.
