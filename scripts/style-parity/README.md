# Style parity

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
bun run style-parity capture --base http://127.0.0.1:5595 \
  --manifest scripts/style-parity/public.manifest.ts --out .bldr/parity-before
bun run style-parity capture --base http://127.0.0.1:5595 \
  --manifest scripts/style-parity/public.manifest.ts --out .bldr/parity-after
bun run style-parity compare --before .bldr/parity-before \
  --after .bldr/parity-after --out .bldr/parity-differences.jsonl
```

Use a new output directory for each capture. The tool saves screenshots,
resolved CSS properties, generated pseudo-elements, geometry and accessible
content for every state in Chromium and WebKit at three sizes. It excludes
custom property declarations because different compilers use different internal
variables; their effects remain visible in the resolved CSS properties.

A manifest exports `states`. Each state has an `id`, `path`, and `ready`, plus
an optional `setup(page)` function that opens the required state. `ready` is
either a selector for the final state or an async function that waits for the
application's own readiness. A heading appearing during loading is insufficient.
Each state starts in a fresh context, so setup must establish its own disposable
fixture. The starter manifest covers three public pages; add session viewers
and overlays before treating it as full application coverage.
`routes.manifest.ts` is a draft route inventory awaiting validation through the
local CDN build.

The capture fixes wall-clock dates, waits for fonts and image decoding, and
freezes animation progress. Finite animations reach their end; repeating
animations stop at their start. The screenshot disables animation and hides the
caret. This records static styles and geometry; it does not establish animation
timing or interaction quality.

The final `capture.json` appears only after every state succeeds. Failed states
save an error record and a diagnostic screenshot and accessibility tree when
the page can still supply them. The tool continues with the remaining states,
then fails the command. Comparison requires the same manifest and capture matrix
and exits with status 1 when any value differs. The JSONL report streams one
pair of screens at a time.

Run `bun run test:style-parity` for the repeatability and single-property check,
and `bun run check:style-parity` and `bun run lint:style-parity` for source checks.
Set `STYLE_PARITY_TEST_ARTIFACTS` to a directory to retain the test's captures.
