import React from 'react'
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  RenderMode,
  type WebDocument as BldrWebDocument,
  type WebView as BldrWebView,
} from '@aptre/bldr'

import { resetStartupMarksForTest } from '../bldr/startup-marks.js'
import { SimpleEventEmitter } from '../bldr/simple-event-emitter.js'
import { WebView } from './WebView.js'

declare global {
  var __webViewReadyCallbacks: Array<() => void> | undefined
}

vi.mock('./web-view-react.js', async () => {
  const ReactModule = await import('react')
  return {
    ReactComponentContainer: ({
      onReady,
      scriptPath,
    }: {
      onReady?: () => void
      scriptPath: string
    }) => {
      if (onReady) {
        globalThis.__webViewReadyCallbacks = [
          ...(globalThis.__webViewReadyCallbacks ?? []),
          onReady,
        ]
      }
      return ReactModule.createElement('div', {
        'data-testid': 'react-component',
        'data-script-path': scriptPath,
      })
    },
  }
})
interface RuntimePresentationEvents {
  [event: string]: (...args: unknown[]) => void
  runtimeconnected: () => void
  runtimeinvalidated: (generation?: unknown) => void
}

interface RuntimePresentationTestState {
  connected: boolean
  generation?: string
}

interface CapturedRegistration {
  view?: BldrWebView
  release: ReturnType<typeof vi.fn>
  presentation?: RuntimePresentationTestState
  emitter?: RuntimePresentationEmitter
}

function makeWebDocument(captured: CapturedRegistration): BldrWebDocument {
  return {
    webDocumentUuid: 'doc-1',
    webRuntimeId: 'runtime-1',
    registerWebView: vi.fn((view: BldrWebView) => {
      captured.view = view
      return { release: captured.release }
    }),
  } as unknown as BldrWebDocument
}

class RuntimePresentationEmitter extends SimpleEventEmitter<RuntimePresentationEvents> {
  public dispatch<K extends keyof RuntimePresentationEvents>(
    event: K,
    ...args: Parameters<RuntimePresentationEvents[K]>
  ): void {
    this.emit(event, ...args)
  }
}

function makePresentationDocument(
  captured: CapturedRegistration,
): BldrWebDocument {
  captured.presentation = { connected: false }
  const emitter = new RuntimePresentationEmitter()
  captured.emitter = emitter
  const baseDocument = makeWebDocument(captured)
  const document = {
    ...baseDocument,
    getRuntimePresentationState: () => ({
      ...(captured.presentation as RuntimePresentationTestState),
    }),
    on: <K extends keyof RuntimePresentationEvents>(
      event: K,
      listener: RuntimePresentationEvents[K],
    ): BldrWebDocument => {
      emitter.on(event, listener)
      return baseDocument
    },
    removeListener: <K extends keyof RuntimePresentationEvents>(
      event: K,
      listener: RuntimePresentationEvents[K],
    ): BldrWebDocument => {
      emitter.removeListener(event, listener)
      return baseDocument
    },
  }
  return document as unknown as BldrWebDocument
}

function emitRuntime(
  captured: CapturedRegistration,
  event: keyof RuntimePresentationEvents,
  generation?: string,
): void {
  if (event === 'runtimeconnected') {
    captured.emitter?.dispatch(event)
    return
  }
  captured.emitter?.dispatch(event, generation)
}

function getStartupMarkLabels(): string[] {
  return (globalThis.__swStartupMarks ?? []).map((mark) => mark.label)
}

function getStartupMarks(label: string) {
  return (globalThis.__swStartupMarks ?? []).filter(
    (mark) => mark.label === label,
  )
}

async function expectMarkCount(label: string, count: number): Promise<void> {
  await waitFor(() => {
    expect(getStartupMarks(label)).toHaveLength(count)
  })
}

async function finishRevealLifecycle(
  view: BldrWebView,
  container: HTMLElement,
  scriptPath: string,
  expectedMarkTotal: number,
): Promise<void> {
  await act(async () => {
    await view.setHtmlLinks({
      clear: true,
      setLinks: {
        app: { href: 'data:text/css,body{}', rel: 'stylesheet' },
      },
    })
  })

  const stylesheet = container.querySelector('link[rel="stylesheet"]')
  expect(stylesheet).toBeTruthy()
  await act(async () => {
    fireEvent.load(stylesheet as Element)
  })
  await expectMarkCount('webview.stylesheet-ready', expectedMarkTotal)

  await act(async () => {
    await view.setRenderMode({
      renderMode: RenderMode.RenderMode_REACT_COMPONENT,
      scriptPath,
    })
  })
  await waitFor(() => {
    expect(globalThis.__webViewReadyCallbacks?.length ?? 0).toBeGreaterThan(0)
  })
  await act(async () => {
    globalThis.__webViewReadyCallbacks?.at(-1)?.()
  })
  await expectMarkCount('webview.component-ready', expectedMarkTotal)
  await expectMarkCount('webview.revealed', expectedMarkTotal)
}

describe('WebView startup boundaries', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    resetStartupMarksForTest()
    globalThis.__webViewReadyCallbacks = undefined
  })

  it('emits startup-relevant registration, stylesheet, component, and reveal evidence once', async () => {
    const captured: CapturedRegistration = { release: vi.fn() }
    const webDocument = makeWebDocument(captured)

    const { container } = render(
      React.createElement(WebView, {
        loading: React.createElement('div', {}, 'Loading'),
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )

    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await expectMarkCount('webview.registered', 1)
    await finishRevealLifecycle(
      captured.view as BldrWebView,
      container,
      '/component-a.js',
      1,
    )

    expect(getStartupMarkLabels()).toEqual([
      'webview.registered',
      'webview.stylesheet-ready',
      'webview.component-ready',
      'webview.revealed',
    ])
    expect(getStartupMarks('webview.registered')[0].detail).toMatchObject({
      label: 'webview.registered',
      source: 'webview',
      startupRelevant: true,
      webDocumentId: 'doc-1',
      webRuntimeId: 'runtime-1',
      webViewId: 'root-view',
    })
    expect(getStartupMarks('webview.stylesheet-ready')[0].detail).toMatchObject(
      {
        stylesheetCount: 1,
      },
    )
    expect(getStartupMarks('webview.component-ready')[0].detail).toMatchObject({
      renderMode: RenderMode.RenderMode_REACT_COMPONENT,
      scriptPath: '/component-a.js',
    })
  })

  it('does not duplicate readiness evidence within one reveal lifecycle', async () => {
    const captured: CapturedRegistration = { release: vi.fn() }
    const webDocument = makeWebDocument(captured)

    const { container } = render(
      React.createElement(WebView, {
        loading: React.createElement('div', {}, 'Loading'),
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )

    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await act(async () => {
      await (captured.view as BldrWebView).setHtmlLinks({
        clear: true,
        setLinks: {
          app: { href: 'data:text/css,body{}', rel: 'stylesheet' },
        },
      })
    })

    const stylesheet = container.querySelector('link[rel="stylesheet"]')
    expect(stylesheet).toBeTruthy()
    await act(async () => {
      fireEvent.load(stylesheet as Element)
      fireEvent.load(stylesheet as Element)
    })

    await act(async () => {
      await (captured.view as BldrWebView).setRenderMode({
        renderMode: RenderMode.RenderMode_REACT_COMPONENT,
        scriptPath: '/component-a.js',
      })
    })
    await waitFor(() => {
      expect(globalThis.__webViewReadyCallbacks?.length ?? 0).toBeGreaterThan(0)
    })
    await act(async () => {
      const onReady = globalThis.__webViewReadyCallbacks?.at(-1)
      onReady?.()
      onReady?.()
    })

    await expectMarkCount('webview.registered', 1)
    await expectMarkCount('webview.stylesheet-ready', 1)
    await expectMarkCount('webview.component-ready', 1)
    await expectMarkCount('webview.revealed', 1)
  })

  it('keeps the app revealed when render mode repeats the same component target', async () => {
    const captured: CapturedRegistration = { release: vi.fn() }
    const webDocument = makeWebDocument(captured)

    const { container } = render(
      React.createElement(WebView, {
        loading: React.createElement('div', {}, 'Loading'),
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )

    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await finishRevealLifecycle(
      captured.view as BldrWebView,
      container,
      '/component-a.js',
      1,
    )
    expect(container.textContent).not.toContain('Loading')

    await act(async () => {
      await (captured.view as BldrWebView).setRenderMode({
        renderMode: RenderMode.RenderMode_REACT_COMPONENT,
        scriptPath: '/component-a.js',
      })
    })

    expect(container.textContent).not.toContain('Loading')
    await expectMarkCount('webview.component-ready', 1)
    await expectMarkCount('webview.revealed', 1)
  })

  it('resets reveal evidence across refresh and remount lifecycles', async () => {
    const captured: CapturedRegistration = { release: vi.fn() }
    const webDocument = makeWebDocument(captured)

    const mounted = render(
      React.createElement(WebView, {
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )

    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await expectMarkCount('webview.registered', 1)
    await finishRevealLifecycle(
      captured.view as BldrWebView,
      mounted.container,
      '/component-a.js',
      1,
    )

    await act(async () => {
      await (captured.view as BldrWebView).setRenderMode({
        refresh: true,
        renderMode: RenderMode.RenderMode_REACT_COMPONENT,
        scriptPath: '/component-a.js',
      })
    })
    await waitFor(() => {
      expect(globalThis.__webViewReadyCallbacks?.length ?? 0).toBeGreaterThan(1)
    })
    await act(async () => {
      globalThis.__webViewReadyCallbacks?.at(-1)?.()
    })

    await expectMarkCount('webview.stylesheet-ready', 2)
    await expectMarkCount('webview.component-ready', 2)
    await expectMarkCount('webview.revealed', 2)

    mounted.unmount()
    expect(captured.release).toHaveBeenCalledTimes(1)

    const remounted = render(
      React.createElement(WebView, {
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )

    await expectMarkCount('webview.registered', 2)
    await finishRevealLifecycle(
      captured.view as BldrWebView,
      remounted.container,
      '/component-b.js',
      3,
    )

    await expectMarkCount('webview.stylesheet-ready', 3)
    await expectMarkCount('webview.component-ready', 3)
    await expectMarkCount('webview.revealed', 3)
  })

  it('resets reveal evidence after resetView before the next reveal lifecycle', async () => {
    const captured: CapturedRegistration = { release: vi.fn() }
    const webDocument = makeWebDocument(captured)

    const mounted = render(
      React.createElement(WebView, {
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )

    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await finishRevealLifecycle(
      captured.view as BldrWebView,
      mounted.container,
      '/component-a.js',
      1,
    )

    await act(async () => {
      await (captured.view as BldrWebView).resetView()
    })

    await expectMarkCount('webview.stylesheet-ready', 1)
    await expectMarkCount('webview.component-ready', 1)
    await expectMarkCount('webview.revealed', 1)

    await finishRevealLifecycle(
      captured.view as BldrWebView,
      mounted.container,
      '/component-b.js',
      2,
    )
  })

  it('keeps non-startup WebView evidence on the startup mark channel', async () => {
    const captured: CapturedRegistration = { release: vi.fn() }
    const webDocument = makeWebDocument(captured)

    const { container } = render(
      React.createElement(WebView, {
        uuid: 'nested-view',
        webDocument,
      }),
    )

    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await finishRevealLifecycle(
      captured.view as BldrWebView,
      container,
      '/component-a.js',
      1,
    )

    for (const label of [
      'webview.registered',
      'webview.stylesheet-ready',
      'webview.component-ready',
      'webview.revealed',
    ]) {
      expect(getStartupMarks(label)[0].detail).toMatchObject({
        source: 'webview',
        startupRelevant: false,
        webViewId: 'nested-view',
      })
    }
  })
  it('owns one generation-matched neutral/loading/reveal transition', async () => {
    const captured: CapturedRegistration = { release: vi.fn() }
    const webDocument = makePresentationDocument(captured)
    const mounted = render(
      React.createElement(WebView, {
        loading: React.createElement('div', {}, 'Loading'),
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )

    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await act(async () => {
      await (captured.view as BldrWebView).setRenderMode({
        renderMode: RenderMode.RenderMode_REACT_COMPONENT,
        scriptPath: '/component-a.js',
      })
    })
    await waitFor(() => {
      expect(globalThis.__webViewReadyCallbacks?.length ?? 0).toBeGreaterThan(0)
    })
    expect(mounted.container.textContent).toContain('Loading')
    expect(getStartupMarks('webview.revealed')).toHaveLength(0)

    captured.presentation = {
      connected: true,
      generation: 'generation-1',
    }
    await act(async () => {
      emitRuntime(captured, 'runtimeconnected')
    })
    await expectMarkCount('webview.neutral-frame', 1)
    expect(mounted.container.textContent).toContain('Loading')
    const mountedComponent = mounted.container.querySelector(
      '[data-testid="react-component"]',
    ) as HTMLElement | null
    expect(mountedComponent).toBeTruthy()
    expect(mountedComponent?.style.display).not.toBe('none')

    await act(async () => {
      globalThis.__webViewReadyCallbacks?.at(-1)?.()
    })
    await expectMarkCount('webview.revealed', 1)
    expect(mounted.container.textContent).not.toContain('Loading')

    await act(async () => {
      emitRuntime(captured, 'runtimeconnected')
    })
    expect(getStartupMarks('webview.neutral-frame')).toHaveLength(1)
    expect(getStartupMarks('webview.revealed')).toHaveLength(1)

    captured.presentation = {
      connected: false,
      generation: 'generation-1',
    }
    await act(async () => {
      emitRuntime(captured, 'runtimeinvalidated', 'generation-1')
    })
    await waitFor(() => {
      expect(mounted.container.textContent).toContain('Loading')
    })

    captured.presentation = {
      connected: true,
      generation: 'generation-2',
    }
    await act(async () => {
      emitRuntime(captured, 'runtimeconnected')
    })
    await expectMarkCount('webview.neutral-frame', 2)
    await expectMarkCount('webview.revealed', 2)
    await act(async () => {
      emitRuntime(captured, 'runtimeinvalidated', 'generation-1')
    })
    expect(mounted.container.textContent).not.toContain('Loading')

    await act(async () => {
      await (captured.view as BldrWebView).resetView()
    })
    await act(async () => {
      await (captured.view as BldrWebView).setRenderMode({
        renderMode: RenderMode.RenderMode_REACT_COMPONENT,
        scriptPath: '/component-b.js',
      })
    })
    await waitFor(() => {
      expect(globalThis.__webViewReadyCallbacks?.length ?? 0).toBeGreaterThan(1)
    })
    await act(async () => {
      globalThis.__webViewReadyCallbacks?.at(-1)?.()
    })
    await expectMarkCount('webview.revealed', 3)

    mounted.unmount()
    const remounted = render(
      React.createElement(WebView, {
        loading: React.createElement('div', {}, 'Loading'),
        startupProgress: true,
        uuid: 'root-view',
        webDocument,
      }),
    )
    await waitFor(() => {
      expect(captured.view).toBeTruthy()
    })
    await act(async () => {
      await (captured.view as BldrWebView).setRenderMode({
        renderMode: RenderMode.RenderMode_REACT_COMPONENT,
        scriptPath: '/component-c.js',
      })
    })
    await waitFor(() => {
      expect(globalThis.__webViewReadyCallbacks?.length ?? 0).toBeGreaterThan(2)
    })
    await act(async () => {
      globalThis.__webViewReadyCallbacks?.at(-1)?.()
    })
    await expectMarkCount('webview.revealed', 4)
    remounted.unmount()
  })
})
