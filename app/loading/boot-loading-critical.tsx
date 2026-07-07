// Critical CSS for the browser boot loading surface.
//
// The WebView shows its `loading` node (AppLoadingScreen) precisely while the
// plugin stylesheets - including the Tailwind app.css that every utility class
// resolves against - are still downloading (see bldr/web/bldr-react/WebView.tsx
// cssLoaded gating). A boot surface built from Tailwind utilities therefore
// paints unstyled during the multi-second bundle download. This stylesheet
// makes the boot surface self-contained: semantic `.swb-*` classes that resolve
// against the design tokens when app.css is present and fall back to the
// baked-in hex values when it is not, so the screen is branded from the first
// paint regardless of stylesheet timing. Values mirror the static #sw-loading
// shell in app/prerender/build.ts.

const MONO_STACK = 'ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace'

export const BOOT_LOADING_CRITICAL_CSS = `
.swb-canvas{display:flex;align-items:center;justify-content:center;flex:1 1 0%;width:100%;height:100%;min-height:0;overflow:hidden;background:var(--color-background,#0a0a0a);color:var(--color-foreground,#fafafa);font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
.swb-col{display:flex;flex-direction:column;align-items:center;gap:1.25rem;text-align:center;width:min(30rem,calc(100vw - 2rem))}
.swb-logo{display:flex;height:5rem;width:5rem;align-items:center;justify-content:center;border-radius:1rem;background:color-mix(in srgb,var(--color-brand,#4f8cff) 10%,transparent);box-shadow:0 0 42px color-mix(in srgb,var(--color-brand,#4f8cff) 22%,transparent)}
.swb-head{display:flex;flex-direction:column;align-items:center;gap:0.5rem}
.swb-title{margin:0;font-size:1.5rem;font-weight:600;letter-spacing:-0.01em;line-height:1.2;color:var(--color-foreground,#fafafa)}
.swb-detail{margin:0;font-size:0.875rem;line-height:1.4;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)}
.swb-hint{margin:0;max-width:22rem;font-size:0.75rem;line-height:1.5;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 52%,transparent)}
.swb-progress-wrap{margin-top:0.25rem;display:flex;width:min(16rem,calc(100vw - 4rem));align-items:center;gap:0.75rem}
.swb-bar{position:relative;height:0.375rem;flex:1;overflow:hidden;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 8%,transparent)}
.swb-bar-fill{height:100%;border-radius:9999px;background:var(--color-brand,#4f8cff);transition:width 200ms ease}
.swb-bar-fill--indeterminate{position:absolute;top:0;bottom:0;left:0;width:33%;animation:swb-indeterminate 1.1s ease-in-out infinite}
.swb-mono{font-family:${MONO_STACK};font-variant-numeric:tabular-nums}
.swb-progress-label{width:2.75rem;text-align:right;font-size:0.7rem;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)}
.swb-rail{position:relative;width:100%}
.swb-rail-track{position:absolute;left:10%;right:10%;top:0.75rem;height:1px;transform:translateY(-50%);overflow:hidden;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent);pointer-events:none}
.swb-rail-fill{height:100%;border-radius:9999px;background:var(--color-brand,#4f8cff);transition:width 500ms ease}
.swb-rail-fill--error{background:color-mix(in srgb,var(--color-destructive,#ef4444) 70%,transparent)}
.swb-steps{position:relative;display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:0.5rem;margin:0;padding:0;list-style:none}
.swb-step{min-width:0}
.swb-step-mark{display:flex;height:1.5rem;align-items:center;justify-content:center}
.swb-dot{display:block;box-sizing:border-box;height:0.625rem;width:0.625rem;border-radius:9999px;transition:all 300ms ease}
.swb-step[data-state="pending"] .swb-dot{border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 25%,transparent);background:var(--color-background,#0a0a0a)}
.swb-step[data-state="complete"] .swb-dot{background:var(--color-brand,#4f8cff)}
.swb-step[data-state="error"] .swb-dot{height:0.75rem;width:0.75rem;background:var(--color-destructive,#ef4444);box-shadow:0 0 0 4px color-mix(in srgb,var(--color-destructive,#ef4444) 25%,transparent)}
.swb-spinner{display:block;box-sizing:border-box;height:0.875rem;width:0.875rem;border-radius:9999px;border:2px solid color-mix(in srgb,var(--color-brand,#4f8cff) 25%,transparent);border-top-color:var(--color-brand,#4f8cff);animation:swb-spin 0.7s linear infinite}
.swb-step-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:center;font-size:0.65rem;font-weight:500;transition:color 300ms ease}
.swb-step[data-state="pending"] .swb-step-label{color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 55%,transparent)}
.swb-step[data-state="complete"] .swb-step-label{color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 85%,transparent)}
.swb-step[data-state="current"] .swb-step-label{color:var(--color-foreground,#fafafa)}
.swb-step[data-state="error"] .swb-step-label{color:var(--color-destructive,#ef4444)}
.swb-downloads{display:flex;width:100%;flex-direction:column;gap:0.75rem;margin:0;padding:0;list-style:none}
.swb-dl-row{display:flex;width:100%;flex-direction:column;gap:0.25rem}
.swb-dl-head{display:flex;align-items:center;justify-content:space-between;gap:0.5rem;font-size:0.7rem}
.swb-dl-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:500;color:color-mix(in srgb,var(--color-foreground,#fafafa) 85%,transparent)}
.swb-dl-detail{flex-shrink:0;font-size:0.7rem;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)}
.swb-dl-detail--error{color:var(--color-destructive,#ef4444)}
.swb-error{margin:0.25rem 0 0;max-width:20rem;border:1px solid color-mix(in srgb,var(--color-destructive,#ef4444) 15%,transparent);border-radius:0.375rem;background:color-mix(in srgb,var(--color-destructive,#ef4444) 5%,transparent);padding:0.5rem 0.75rem;font-size:0.75rem;line-height:1.5;color:var(--color-destructive,#ef4444)}
.swb-actions{display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:0.5rem;margin-top:0.25rem}
.swb-btn{display:inline-flex;align-items:center;gap:0.5rem;height:2.25rem;padding:0 0.75rem;border-radius:0.375rem;font-size:0.875rem;font-weight:500;cursor:pointer;transition:background 150ms ease,border-color 150ms ease,color 150ms ease}
.swb-btn--primary{border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent);background:color-mix(in srgb,var(--color-foreground,#fafafa) 5%,transparent);color:var(--color-foreground,#fafafa)}
.swb-btn--ghost{border:0;background:transparent;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 80%,transparent)}
@keyframes swb-spin{to{transform:rotate(360deg)}}
@keyframes swb-indeterminate{0%{left:-33%}100%{left:100%}}
@media (prefers-reduced-motion:reduce){.swb-spinner,.swb-bar-fill--indeterminate{animation:none}.swb-bar-fill--indeterminate{left:0;width:100%}.swb-dot,.swb-step-label,.swb-bar-fill,.swb-rail-fill,.swb-btn{transition:none}}
`.trim()

// BootLoadingCriticalStyle inlines the boot critical CSS into the document.
// React 19 hoists a keyed <style href precedence> into <head> and dedupes it,
// so multiple boot surfaces (AppLoadingScreen, the quickstart phase rail) share
// a single injected stylesheet. The style is inlined rather than linked so it
// is applied without a network round-trip during the boot window.
export function BootLoadingCriticalStyle() {
  return (
    <style href="sw-boot-loading-critical" precedence="high">
      {BOOT_LOADING_CRITICAL_CSS}
    </style>
  )
}
