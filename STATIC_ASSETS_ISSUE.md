### Preventing asset inlining when using Vite **library mode** in *bldr*

#### 1. What we are seeing today

* Every build that goes through `web/bundler/vite/vite.ts` ends up enabling `build.lib` so that we can have **multiple JS entry-points** that still share code.  You can see the library-mode branch being built here:

```120:140:web/bundler/vite/vite.ts
// ... existing code ...
```

* The example project mirrors the same approach explicitly:

```1:23:example/vite.config.ts
// ... existing code ...
```

* As soon as `build.lib` is defined, Vite internally switches to **library mode**.  In that mode the built-in `vite:asset` plugin always converts every imported asset (images, fonts, wasm, …) into an **in-memory base-64 data-URI** and injects it into the JS/CSS bundle.  The `build.assetsInlineLimit` knob is **ignored** (see vitejs/vite#4454).
  * Drawbacks we observe:
    * release bundles become very large (a single font can add > 100 kB gzip).
    * CDN caching of long-lived assets is impossible because they are hidden inside the JS file hash.

#### 2. Why Vite does this

Vite's current reasoning is that a library published to npm should be fully self-contained.  Emitting separate files would force downstream consumers to copy those assets next to the library on their own.  For *applications* (our use case) this behaviour is actually harmful.

#### 3. Options to get **multiple entry-points + shared chunks** **without** asset inlining

| Solution | Effort | Pros | Cons |
|----------|--------|------|------|
| **A. Leave library mode & patch it** with a small custom Vite plugin (`no-inline-assets`) that runs *after* `vite:asset` and overwrites the generated module code so it exports `new URL('file.ext', import.meta.url).href` instead of the base-64 string | ⭐ 1-2 h | • minimal change<br/>• keeps current bundler pipeline | • still loses CSS code-splitting in lib mode<br/>• needs PostCSS fix for `url()` in CSS |
| **B. Drop library mode** and treat the project as a regular multi-entry build by using `build.rollupOptions.input` instead of `build.lib`  | ⭐⭐ 2-4 h | • Vite will emit assets & CSS files correctly out of the box<br/>• still generates shared `vendor-*.js` chunk automatically | • output filenames differ (not `index.es.js`) – may require small import-map tweaks |
| **C. Wait for Vite PR #9734** (`emitAssetsWithModule`) to land and upgrade | ❓ | • official upstream support | • timeline uncertain

#### 4. Recommended path

Short-term we can ship **Solution B** because it needs no hacks and keeps Vite on the happy path:

```ts
// pseudo-patch inside web/bundler/vite/vite.ts
// Replace the lib branch with rollup input:
mergedConfig.build.rollupOptions = {
  ...mergedConfig.build.rollupOptions,
  input: entrypoints.reduce((acc, e) => {
    const name = path.basename(e.inputPath, path.extname(e.inputPath))
    acc[name] = e.inputPath
    return acc
  }, {} as Record<string,string>),
}
// Remove mergedConfig.build.lib entirely
```

Additional flags that should be set once library mode is gone:
```ts
build: {
  assetsInlineLimit: 0,          // just to be explicit
  cssCodeSplit: true,
  rollupOptions: {
    output: {
      assetFileNames: "assets/[name]-[hash][extname]",
    },
  },
}
```

Long-term we can revisit **Solution C** once the feature ships upstream.

#### 5. If we really must stay in library mode (Solution A)

Create `web/bundler/vite/plugins/no-inline-assets.ts`:
```ts
import { relative, dirname } from 'path'
import type { Plugin } from 'vite'

export function noInlineAssets(): Plugin {
  const re = /\.(png|jpe?g|gif|svg|webp|ico|bmp|tiff|woff2?|eot|ttf|otf|wasm)$/i
  return {
    name: 'no-inline-assets',
    enforce: 'post',
    load(id) {
      if (!re.test(id)) return null
      const rel = relative(dirname(id), id).replace(/\\/g, '/')
      return `export default new URL(${JSON.stringify('./' + rel)}, import.meta.url).href;`
    },
  }
}
```
and push it into `mergedConfig.plugins` *after* all default Vite plugins.

For assets referenced from CSS we can couple this with `postcss-url` set to `url: 'copy'` so that font files are emitted as separate files too.

---

### TL;DR
By switching away from library mode (or by patching it with a tiny plugin) we can restore normal asset emission and keep the desired shared-chunk behaviour.  Until vitejs/vite#4454 is fixed upstream this is the most robust path for *bldr*. 