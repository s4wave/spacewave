import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  browserStartupMarkEvent,
  browserStartupMarkPrefix,
  canMutateBrowserBootStatusTarget,
  markBrowserStartupBoundary,
  readBrowserBootStatus,
  readBrowserStartupMarks,
  resetBrowserStartupMarksForTest,
  writeBrowserBootStatus,
} from './boot-status.js'

afterEach(() => {
  document.body.innerHTML = ''
  globalThis.__swBootStatus = undefined
  resetBrowserStartupMarksForTest()
  vi.restoreAllMocks()
})

describe('canMutateBrowserBootStatusTarget', () => {
  it('allows boot status updates outside prerendered React roots', () => {
    document.body.innerHTML = '<p data-sw-boot-status></p>'

    expect(
      canMutateBrowserBootStatusTarget(
        document.querySelector('[data-sw-boot-status]'),
      ),
    ).toBe(true)
  })

  it('blocks boot status updates inside prerendered React-owned pages', () => {
    document.body.innerHTML = `
      <div id="bldr-root" data-prerendered="true">
        <p data-sw-boot-status></p>
      </div>
    `

    expect(
      canMutateBrowserBootStatusTarget(
        document.querySelector('[data-sw-boot-status]'),
      ),
    ).toBe(false)
  })

  it('allows boot status updates for the non-hydrated root loading screen', () => {
    document.body.innerHTML = `
      <div id="bldr-root" data-prerendered="true">
        <div id="sw-loading">
          <p data-sw-boot-status></p>
        </div>
      </div>
    `

    expect(
      canMutateBrowserBootStatusTarget(
        document.querySelector('[data-sw-boot-status]'),
      ),
    ).toBe(true)
  })
})

describe('writeBrowserBootStatus', () => {
  it('stores raw boot status and updates static DOM with projected startup copy', () => {
    document.body.innerHTML = `
      <p data-sw-boot-status>Loading application...</p>
      <div data-sw-boot-progress></div>
      <span data-sw-boot-progress-label></span>
    `

    writeBrowserBootStatus({
      phase: 'wasm',
      detail: 'Preparing runtime...',
      state: 'loading',
    })

    expect(readBrowserBootStatus()).toEqual({
      phase: 'wasm',
      detail: 'Preparing runtime...',
      state: 'loading',
      progress: 0.38,
    })
    expect(document.querySelector('[data-sw-boot-status]')?.textContent).toBe(
      'Connect: Connecting the app shell.',
    )
    const progress = document.querySelector('[data-sw-boot-progress]')
    if (!(progress instanceof HTMLElement)) {
      throw new Error('missing boot progress target')
    }
    expect(progress.style.width).toBe('30%')
    expect(progress.getAttribute('aria-valuenow')).toBe('30')
    expect(
      document.querySelector('[data-sw-boot-progress-label]')?.textContent,
    ).toBe('30%')
  })

  it('uses indeterminate static progress while the app bundle is downloading', () => {
    document.body.innerHTML = `
      <p data-sw-boot-status>Loading application...</p>
      <div data-sw-boot-progress aria-valuemin="0" aria-valuemax="100"></div>
      <span data-sw-boot-progress-label></span>
    `
    markBrowserStartupBoundary('shell.boot-requested', { source: 'test' })

    writeBrowserBootStatus({
      phase: 'runtime',
      detail: 'Starting runtime...',
      state: 'loading',
    })

    expect(document.querySelector('[data-sw-boot-status]')?.textContent).toBe(
      'App: Downloading the app bundle. This can take a while the first time.',
    )
    const progress = document.querySelector('[data-sw-boot-progress]')
    if (!(progress instanceof HTMLElement)) {
      throw new Error('missing boot progress target')
    }
    expect(progress.classList.contains('animate-progress-indeterminate')).toBe(
      true,
    )
    expect(progress.style.width).toBe('33%')
    expect(progress.getAttribute('aria-valuenow')).toBeNull()
    expect(progress.getAttribute('aria-valuetext')).toBe('Loading')
    expect(
      document.querySelector('[data-sw-boot-progress-label]')?.textContent,
    ).toBe('')
  })

  it('updates static shell phase state from the projected startup rail', () => {
    document.body.innerHTML = `
      <div id="sw-loading">
        <ol>
          <li data-sw-boot-phase="prepare" data-sw-boot-phase-state="current">
            <span data-sw-boot-phase-dot></span>
            <span data-sw-boot-phase-label>Prepare</span>
          </li>
          <li data-sw-boot-phase="connect" data-sw-boot-phase-state="pending">
            <span data-sw-boot-phase-dot></span>
            <span data-sw-boot-phase-label>Connect</span>
          </li>
          <li data-sw-boot-phase="runtime" data-sw-boot-phase-state="pending">
            <span data-sw-boot-phase-dot></span>
            <span data-sw-boot-phase-label>Runtime</span>
          </li>
        </ol>
      </div>
    `

    writeBrowserBootStatus({
      phase: 'runtime',
      detail: 'Connecting runtime...',
      state: 'loading',
    })

    expect(
      document
        .querySelector('[data-sw-boot-phase="prepare"]')
        ?.getAttribute('data-sw-boot-phase-state'),
    ).toBe('complete')
    expect(
      document
        .querySelector('[data-sw-boot-phase="connect"]')
        ?.getAttribute('data-sw-boot-phase-state'),
    ).toBe('complete')
    expect(
      document
        .querySelector('[data-sw-boot-phase="runtime"]')
        ?.getAttribute('data-sw-boot-phase-state'),
    ).toBe('current')
  })

  it('renders recoverable static shell error actions', () => {
    document.body.innerHTML = `
      <div id="sw-loading">
        <p data-sw-boot-status>Loading application...</p>
        <p data-sw-boot-error style="display:none"></p>
        <div data-sw-boot-error-actions style="display:none">
          <button data-sw-boot-retry type="button">Retry</button>
          <button data-sw-boot-back type="button">Back</button>
        </div>
      </div>
    `
    const reload = vi.fn()
    const assign = vi.fn()
    vi.stubGlobal('location', {
      ...window.location,
      assign,
      reload,
    })

    writeBrowserBootStatus({
      phase: 'runtime-error',
      detail: 'runtime failed',
      state: 'error',
    })

    const error = document.querySelector('[data-sw-boot-error]') as HTMLElement
    const actions = document.querySelector(
      '[data-sw-boot-error-actions]',
    ) as HTMLElement
    expect(error.textContent).toBe(
      'Startup did not finish. Check the browser console or startup marks for details.',
    )
    expect(error.style.display).toBe('')
    expect(actions.style.display).toBe('flex')
    ;(
      document.querySelector('[data-sw-boot-retry]') as HTMLButtonElement
    ).click()
    expect(reload).toHaveBeenCalledTimes(1)
    expect(readBrowserStartupMarks().at(-1)?.label).toBe('boot-status.retry')
  })

  it('emits startup marks for boot status changes', () => {
    const mark = vi.spyOn(performance, 'mark').mockImplementation(() => {
      return {} as PerformanceMark
    })
    const listener = vi.fn()
    window.addEventListener(browserStartupMarkEvent, listener)

    writeBrowserBootStatus({
      phase: 'entrypoint',
      detail: 'Starting application...',
      state: 'loading',
    })

    window.removeEventListener(browserStartupMarkEvent, listener)
    expect(mark).toHaveBeenCalledWith(
      `${browserStartupMarkPrefix}boot-status.entrypoint`,
      {
        detail: {
          label: 'boot-status.entrypoint',
          sequence: 1,
          source: 'browser',
          phase: 'entrypoint',
          state: 'loading',
          progress: 0.54,
        },
      },
    )
    expect(listener).toHaveBeenCalledTimes(1)
    expect(readBrowserStartupMarks()).toEqual([
      {
        name: `${browserStartupMarkPrefix}boot-status.entrypoint`,
        label: 'boot-status.entrypoint',
        sequence: 1,
        detail: {
          label: 'boot-status.entrypoint',
          sequence: 1,
          source: 'browser',
          phase: 'entrypoint',
          state: 'loading',
          progress: 0.54,
        },
      },
    ])
  })

  it('continues an existing global startup mark sequence', () => {
    globalThis.__swStartupMarkSequence = 4

    writeBrowserBootStatus({
      phase: 'runtime',
      detail: 'Connecting runtime...',
      state: 'loading',
    })

    expect(readBrowserStartupMarks()[0]).toMatchObject({
      label: 'boot-status.runtime',
      sequence: 4,
      detail: {
        sequence: 4,
      },
    })
    expect(globalThis.__swStartupMarkSequence).toBe(5)
  })
})
