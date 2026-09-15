// BOOT_LOADING_CRITICAL_CSS styles startup diagnostics before app.css loads.
// The shared LoadingScreen stylesheet owns the surrounding boot composition.

const MONO_STACK = 'ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace'

export const BOOT_LOADING_CRITICAL_CSS = `
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
@keyframes swb-spin{to{transform:rotate(360deg)}}
@keyframes swb-indeterminate{0%,100%{left:0}50%{left:67%}}
@media (prefers-reduced-motion:reduce){.swb-spinner,.swb-bar-fill--indeterminate{animation:none}.swb-bar-fill--indeterminate{left:33%;width:33%}.swb-dot,.swb-step-label,.swb-bar-fill,.swb-rail-fill{transition:none}}
`.trim()

// BootLoadingCriticalStyle inlines the boot critical CSS into the document.
// React 19 hoists a keyed <style href precedence> into <head> and dedupes it,
// so multiple boot surfaces (AppLoadingScreen, the quickstart phase rail) share
// a single injected stylesheet. The style is inlined rather than linked so it
// is applied without a network round-trip during the boot window.
export function BootLoadingCriticalStyle() {
  return null
}
