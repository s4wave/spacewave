import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  GoWasmProcess,
  patchTinyGoRuntimeImports,
  type TinyGoRuntime,
} from './go-process.js'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('patchTinyGoRuntimeImports', () => {
  it('adds TinyGo random data import backed by wasm memory', () => {
    const memory = new WebAssembly.Memory({ initial: 1 })
    const getRandomValues = vi.fn((view: Uint8Array) => {
      view.fill(7)
      return view
    })
    vi.stubGlobal('crypto', { getRandomValues })

    const go: TinyGoRuntime = {
      importObject: {
        gojs: {},
      },
      _inst: {
        exports: {
          memory,
        },
      },
    }

    patchTinyGoRuntimeImports(go)
    const getRandomData = go.importObject['gojs']?.['runtime.getRandomData']
    if (typeof getRandomData !== 'function') {
      throw new Error('runtime.getRandomData was not installed')
    }

    getRandomData(12, 4, 16)

    expect(getRandomValues).toHaveBeenCalledWith(expect.any(Uint8Array))
    expect(Array.from(new Uint8Array(memory.buffer, 12, 4))).toEqual([
      7, 7, 7, 7,
    ])
  })

  it('keeps an existing random data import', () => {
    const getRandomData = vi.fn()
    const go: TinyGoRuntime = {
      importObject: {
        gojs: {
          'runtime.getRandomData': getRandomData,
        },
      },
    }

    patchTinyGoRuntimeImports(go)

    expect(go.importObject['gojs']?.['runtime.getRandomData']).toBe(
      getRandomData,
    )
  })
})

describe('GoWasmProcess', () => {
  it('suppresses Go released-callback console errors in worker-hosted WASM', async () => {
    const run = vi.fn(async () => {
      console.error('call to released function')
      console.error('other failure')
    })
    class FakeGo {
      public readonly importObject = {}
      public env: Record<string, string> = {}
      public argv: string[] = []
      public run = run
    }

    vi.stubGlobal('Go', FakeGo)
    vi.spyOn(WebAssembly, 'instantiate').mockResolvedValue(
      {} as WebAssembly.Instance,
    )
    const consoleError = vi.spyOn(console, 'error')

    const process = new GoWasmProcess({}, {
      retry: false,
    })
    await process.start()

    expect(consoleError).toHaveBeenCalledTimes(1)
    expect(consoleError).toHaveBeenCalledWith('other failure')
  })
})
