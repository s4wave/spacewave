import { LOADING_SCREEN_CSS } from '@s4wave/web/ui/loading/loading-screen-style.js'

import { BOOT_LOADING_CRITICAL_CSS } from '../loading/boot-loading-critical.js'
import { projectBrowserStartup } from '../loading/status/browser-startup-model.js'

import { ROOT_LOADING_STYLE } from './root-loading-shell.js'

// buildStartupShell renders usable startup feedback before JavaScript loads.
// The boot projection owns phase updates and recovery actions.
export function buildStartupShell(iconUrl: string): string {
  const escapedIcon = iconUrl
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
  const { phases } = projectBrowserStartup({
    phase: 'boot',
    state: 'loading',
    detail: '',
  })
  const rail = phases
    .map(
      (phase) =>
        `<li class="swb-step" data-sw-boot-phase="${phase.id}" data-sw-boot-phase-state="${phase.state}"><div class="swb-step-mark"><span data-sw-boot-phase-dot class="swb-dot" aria-hidden="true"></span></div><div data-sw-boot-phase-label class="swb-step-label">${phase.label}</div></li>`,
    )
    .join('')
  return `<style>${LOADING_SCREEN_CSS}\n${BOOT_LOADING_CRITICAL_CSS}</style>
    <div id="sw-loading" data-sw-boot-state="loading" style="${ROOT_LOADING_STYLE}">
      <div class="swl-canvas">
        <div class="swl-main">
          <div class="swl-emblem" aria-hidden="true">
            <img src="${escapedIcon}" alt="" width="56" height="56"/>
          </div>
          <div class="swl-console">
            <div class="swl-brand" aria-hidden="true">Spacewave</div>
            <div class="swl-head" aria-live="polite" aria-atomic="true">
              <h2 class="swl-title swl-boot-title">Preparing Spacewave</h2>
              <h2 class="swl-title swl-boot-error-title">Unable to open Spacewave</h2>
            </div>
            <div class="swl-phases"><ol class="swb-steps" aria-label="Startup phases">${rail}</ol></div>
            <p data-sw-boot-status hidden>Loading the app shell.</p>
            <p data-sw-boot-error class="swl-error" role="alert" style="display:none"></p>
            <div class="swl-footer">
              <div data-sw-boot-error-actions style="display:none">
                <button data-sw-boot-retry type="button" class="swl-action swl-action--primary">Retry</button>
              </div>
              <button data-sw-boot-back type="button" class="swl-action">Back</button>
            </div>
          </div>
        </div>
      </div>
    </div>`
}
