import { BOOT_LOADING_CRITICAL_CSS } from '../loading/boot-loading-critical.js'

import { ROOT_LOADING_STYLE } from './root-loading-shell.js'

// buildStartupShell renders the first loading surface with the same critical
// styles as the React screen. The boot projection updates diagnostic evidence
// without turning readiness milestones into download percentages.
export function buildStartupShell(iconUrl: string): string {
  const escapedIcon = iconUrl
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
  return `<style>${BOOT_LOADING_CRITICAL_CSS}</style>
    <div id="sw-loading" data-sw-boot-state="loading" style="${ROOT_LOADING_STYLE}">
      <div class="swb-canvas">
        <div class="swb-col">
          <img class="swb-logo" src="${escapedIcon}" alt="" width="120" height="120"/>
          <div class="swb-head" aria-live="polite">
            <h1 class="swb-title">Opening Spacewave</h1>
            <p class="swb-detail">Starting the app</p>
          </div>
          <div class="swb-activity swb-bar" role="progressbar" aria-label="Opening Spacewave">
            <div class="swb-bar-fill swb-bar-fill--indeterminate"></div>
          </div>
          <p class="swb-hint">Downloaded files are saved on this device.</p>
          <p data-sw-boot-error class="swb-error" role="alert" style="display:none"></p>
          <div data-sw-boot-error-actions class="swb-actions" style="display:none">
            <button data-sw-boot-retry type="button" class="swb-btn swb-btn--primary">Retry</button>
            <button data-sw-boot-back type="button" class="swb-btn swb-btn--ghost">Back</button>
          </div>
          <details class="swb-disclosure">
            <summary>Show details</summary>
            <div class="swb-diagnostics">
              <p data-sw-boot-status class="swb-status">Loading the app shell.</p>
              <div data-sw-boot-downloads class="swb-downloads" style="display:none"></div>
            </div>
          </details>
        </div>
      </div>
    </div>`
}
