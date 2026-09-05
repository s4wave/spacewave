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
// paint regardless of stylesheet timing. The static #sw-loading shell imports
// this same stylesheet.

const MONO_STACK = 'ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace'

export const BOOT_LOADING_CRITICAL_CSS = `
.swb-canvas{display:flex;align-items:center;justify-content:center;flex:1 1 0%;box-sizing:border-box;width:100%;height:100%;min-height:0;overflow:auto;padding:48px 24px;background:var(--color-background,#181617);color:var(--color-foreground,#faf8f9);font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
.swb-col{display:flex;flex-direction:column;align-items:center;text-align:center;width:min(440px,100%);margin:auto}
.swb-logo{display:block;flex:none;height:120px;width:120px;object-fit:contain;margin-bottom:44px}
.swb-head{display:flex;flex-direction:column;align-items:center;gap:16px;width:100%}
.swb-title{margin:0;font-size:36px;font-weight:600;letter-spacing:-1px;line-height:1.2;color:var(--color-foreground,#faf8f9)}
.swb-detail{margin:0;font-size:18px;line-height:1.5;color:var(--color-foreground-alt,#aaa4a8)}
.swb-hint{margin:20px 0 0;font-size:14px;line-height:1.5;color:var(--color-foreground-alt,#aaa4a8)}
.swb-bar{position:relative;height:6px;flex:none;overflow:hidden;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent)}
.swb-bar-fill{height:100%;border-radius:9999px;background:var(--color-brand,#e96093);transition:width 200ms ease}
.swb-bar-fill--indeterminate{position:absolute;top:0;bottom:0;left:0;width:33%;animation:swb-indeterminate 1.8s ease-in-out infinite}
.swb-mono{font-family:${MONO_STACK};font-variant-numeric:tabular-nums}
.swb-rail{position:relative;width:100%}
.swb-rail-track{position:absolute;left:10%;right:10%;top:0.75rem;height:1px;transform:translateY(-50%);overflow:hidden;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent);pointer-events:none}
.swb-rail-fill{height:100%;border-radius:9999px;background:var(--color-brand,#e96093);transition:width 500ms ease}
.swb-rail-fill--error{background:color-mix(in srgb,var(--color-destructive,#ef4444) 70%,transparent)}
.swb-steps{position:relative;display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:0.5rem;margin:0;padding:0;list-style:none}
.swb-step{min-width:0}
.swb-step-mark{display:flex;height:1.5rem;align-items:center;justify-content:center}
.swb-dot{display:block;box-sizing:border-box;height:0.625rem;width:0.625rem;border-radius:9999px;transition:all 300ms ease}
.swb-step[data-state="pending"] .swb-dot{border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 25%,transparent);background:var(--color-background,#181617)}
.swb-step[data-state="complete"] .swb-dot{background:var(--color-brand,#e96093)}
.swb-step[data-state="error"] .swb-dot{height:0.75rem;width:0.75rem;background:var(--color-destructive,#ef4444);box-shadow:0 0 0 4px color-mix(in srgb,var(--color-destructive,#ef4444) 25%,transparent)}
.swb-spinner{display:block;box-sizing:border-box;height:0.875rem;width:0.875rem;border-radius:9999px;border:2px solid color-mix(in srgb,var(--color-brand,#e96093) 25%,transparent);border-top-color:var(--color-brand,#e96093);animation:swb-spin 0.7s linear infinite}
.swb-step-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:center;font-size:0.65rem;font-weight:500;transition:color 300ms ease}
.swb-step[data-state="pending"] .swb-step-label{color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 55%,transparent)}
.swb-step[data-state="complete"] .swb-step-label{color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 85%,transparent)}
.swb-step[data-state="current"] .swb-step-label{color:var(--color-foreground,#fafafa)}
.swb-step[data-state="error"] .swb-step-label{color:var(--color-destructive,#ef4444)}
.swb-downloads{display:flex;width:100%;flex-direction:column;gap:0.75rem;margin:0;padding:0;list-style:none}
.swb-dl-row{display:flex;width:100%;flex-direction:column;gap:0.25rem}
.swb-dl-head{display:flex;align-items:flex-start;flex-wrap:wrap;justify-content:space-between;gap:0.5rem;font-size:0.7rem}
.swb-dl-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:500;color:color-mix(in srgb,var(--color-foreground,#fafafa) 85%,transparent)}
.swb-dl-detail{flex-shrink:0;max-width:100%;overflow-wrap:anywhere;font-size:0.7rem;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)}
.swb-dl-detail--error{color:var(--color-destructive,#ef4444)}
.swb-error{margin:24px 0 0;max-width:100%;overflow-wrap:anywhere;border:1px solid color-mix(in srgb,var(--color-destructive,#ef4444) 30%,transparent);border-radius:8px;background:color-mix(in srgb,var(--color-destructive,#ef4444) 5%,transparent);padding:12px 16px;font-size:14px;line-height:1.5;color:var(--color-destructive,#ef4444)}
.swb-actions{display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:0.5rem;margin-top:0.25rem}
.swb-btn{display:inline-flex;align-items:center;gap:0.5rem;height:2.25rem;padding:0 0.75rem;border-radius:0.375rem;font-size:0.875rem;font-weight:500;cursor:pointer;transition:background 150ms ease,border-color 150ms ease,color 150ms ease}
.swb-btn--primary{border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent);background:color-mix(in srgb,var(--color-foreground,#fafafa) 5%,transparent);color:var(--color-foreground,#fafafa)}
.swb-btn--ghost{border:0;background:transparent;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 80%,transparent)}
.swb-activity{width:min(380px,100%);margin-top:36px}
.swb-disclosure{width:100%;margin-top:32px;font-size:14px;color:var(--color-foreground-alt,#aaa4a8)}
.swb-disclosure summary{display:inline-flex;align-items:center;gap:10px;cursor:pointer;padding:8px 12px;list-style:none;border-radius:6px}
.swb-disclosure summary::-webkit-details-marker{display:none}
.swb-disclosure summary::after{content:"";width:6px;height:6px;border-right:1px solid currentColor;border-bottom:1px solid currentColor;transform:rotate(45deg) translateY(-2px)}
.swb-disclosure[open] summary::after{transform:rotate(225deg) translate(-2px,-2px)}
.swb-disclosure summary:hover{color:var(--color-foreground,#faf8f9)}
.swb-disclosure summary:focus-visible,.swb-btn:focus-visible{outline:2px solid var(--color-brand,#e96093);outline-offset:4px}
.swb-diagnostics{margin-top:16px;padding:16px;border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 12%,transparent);border-radius:8px;text-align:left}
.swb-status{margin:0;font-size:13px;line-height:1.5;overflow-wrap:anywhere}
.swb-diagnostics .swb-downloads{margin-top:16px}
[data-sw-boot-state="error"] .swb-activity,[data-sw-boot-state="error"] .swb-detail,[data-sw-boot-state="error"] .swb-hint{display:none}
@media(max-width:600px){.swb-logo{width:80px;height:80px;margin-bottom:32px}.swb-title{font-size:28px;letter-spacing:-.6px}.swb-detail{font-size:16px}.swb-activity{margin-top:28px}.swb-disclosure{margin-top:24px}}
@keyframes swb-spin{to{transform:rotate(360deg)}}
@keyframes swb-indeterminate{0%,100%{left:0}50%{left:67%}}
@media (prefers-reduced-motion:reduce){.swb-spinner,.swb-bar-fill--indeterminate{animation:none}.swb-bar-fill--indeterminate{left:33%;width:33%}.swb-dot,.swb-step-label,.swb-bar-fill,.swb-rail-fill,.swb-btn{transition:none}}
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
