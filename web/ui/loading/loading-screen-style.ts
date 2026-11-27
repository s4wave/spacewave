// LOADING_SCREEN_CSS styles the static and React startup surfaces before
// app.css loads.
export const LOADING_SCREEN_CSS = `
.swl-canvas {
  --app-brand: oklch(0.82 0.1355 16.5);
  --color-brand: var(--app-brand);
  position: relative;
  display: flex;
  flex: 1 1 0%;
  flex-direction: column;
  align-items: center;
  width: 100%;
  height: 100%;
  min-height: 0;
  padding: 24px;
  overflow: auto;
  box-sizing: border-box;
  background: var(--app-background, oklch(0.225 0.002 16.5));
  color: var(--color-foreground, #fafafa);
  font-family: var(--font-display, 'Manrope Variable', sans-serif);
}
.swl-canvas * { box-sizing: border-box; }
.swl-main {
  display: flex;
  flex: none;
  flex-direction: column;
  align-items: center;
  gap: 20px;
  width: 100%;
  max-width: 340px;
  margin: auto;
}
.swl-emblem {
  position: relative;
  display: grid;
  place-items: center;
  flex: none;
  width: 80px;
  height: 80px;
}
.swl-emblem::before {
  content: '';
  position: absolute;
  inset: 0;
  border: 1px solid color-mix(in srgb, var(--app-brand) 15%, transparent);
  border-top-color: color-mix(in srgb, var(--app-brand) 60%, transparent);
  border-radius: 50%;
  animation: swl-orbit 8s linear infinite;
}
.swl-emblem > img, .swl-emblem > svg {
  display: block;
  width: 56px;
  height: 56px;
  object-fit: contain;
  animation: swl-pulse 3s ease-in-out infinite;
}
.swl-console { display: flex; flex-direction: column; gap: 12px; width: 100%; }
.swl-brand { font-size: 10px; font-weight: 500; letter-spacing: .24em; text-transform: uppercase; text-align: center; color: var(--app-brand); }
.swl-title { margin: 0; font-size: 16px; line-height: 24px; font-weight: 500; text-align: center; text-wrap: pretty; }
.swl-detail, .swl-error { margin: 0; font-size: 12px; line-height: 18px; text-align: center; overflow-wrap: anywhere; }
.swl-detail { color: var(--color-foreground-alt, #c3b5c5); }
.swl-error { color: #f1acb2; }
.swl-phases { width: 100%; }
.swl-phases > .space-y-2 { display: flex; flex-direction: column; gap: 8px; padding: 0; width: max-content; max-width: 100%; margin: 0 auto; }
.swl-phases .items-center { display: flex; align-items: center; gap: 9px; }
.swl-phases .size-4, .swl-phases .h-4 { flex: none; width: 16px; height: 16px; }
.swl-phases .size-3 { width: 12px; height: 12px; }
.swl-phases .rounded-full { border-radius: 50%; }
.swl-phases .border { border: 1px solid #ffffff33; }
.swl-phases .bg-brand { background: var(--app-brand); color: var(--app-background, #1d1b1b); justify-content: center; }
.swl-phases .text-brand { color: var(--app-brand); }
.swl-phases .text-xs { font-size: 12px; line-height: 18px; }
.swl-phases .animate-spin { animation: swl-orbit 1.4s linear infinite; }
.swl-transfer { display: flex; justify-content: space-between; font-size: 11px; color: #ffffff9c; }
.swl-transfer-track { height: 3px; overflow: hidden; border-radius: 3px; background: #ffffff12; margin-top: -6px; }
.swl-transfer-track > div { height: 100%; background: var(--app-brand); }
.swl-footer { display: flex; justify-content: center; gap: 18px; font-size: 11px; color: var(--color-foreground-alt, #c3b5c5); }
.swl-footer:empty { display: none; }
.swl-footer a, .swl-action { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 32px; padding: 4px 8px; font: inherit; color: inherit; text-decoration: none; background: transparent; border: 0; border-radius: 4px; cursor: pointer; }
.swl-action svg { height: 14px; width: 14px; }
.swl-footer a:hover, .swl-action:hover { color: #f4f7fc; }
.swl-footer a:focus-visible, .swl-action:focus-visible { outline: 2px solid var(--app-brand); outline-offset: 4px; }
.swl-action--primary { color: var(--app-brand); }
[data-sw-boot-phase-state=pending] .swb-dot { border: 1px solid #ffffff40; background: transparent; }
[data-sw-boot-phase-state=current] .swb-dot { width: 14px; height: 14px; border: 2px solid #efb3c340; border-top-color: var(--app-brand); background: transparent; animation: swl-orbit .7s linear infinite; }
[data-sw-boot-phase-state=complete] .swb-dot { background: var(--app-brand); }
.swl-boot-error-title, [data-sw-boot-state=error] .swl-boot-title { display: none; }
[data-sw-boot-state=error] .swl-boot-error-title { display: block; }
@keyframes swl-orbit { to { transform: rotate(360deg); } }
@keyframes swl-pulse { 50% { opacity: .65; } }
@media (max-height: 400px) {
  .swl-canvas { padding: 16px; }
  .swl-main { gap: 12px; }
  .swl-emblem { width: 56px; height: 56px; }
  .swl-emblem > img, .swl-emblem > svg { width: 40px; height: 40px; }
}
@media (prefers-reduced-motion: reduce) {
  .swl-emblem::before, .swl-emblem > img, .swl-emblem > svg, .swl-phases .animate-spin, [data-sw-boot-phase-state=current] .swb-dot { animation: none; }
}
`.trim()
