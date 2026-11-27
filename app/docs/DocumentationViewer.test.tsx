import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import type { StateNamespace } from '@s4wave/web/state/index.js'
import { useDocPage } from './DocumentationViewer.js'

// file is the page's file handle the mocked hook returns.
const file = vi.hoisted(() => ({ handle: null as unknown }))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: vi.fn(),
  useUnixFSHandle: () => ({ value: file.handle, loading: false, error: null }),
  useUnixFSHandleEntries: vi.fn(),
  useUnixFSHandleTextContent: () => ({
    value: 'original',
    loading: false,
    error: null,
  }),
}))
vi.mock('@s4wave/web/state/index.js', () => ({
  useStateNamespace: vi.fn(),
  useStateAtom: <T,>(_ns: unknown, key: string, initial: T) =>
    // eslint-disable-next-line react-hooks/rules-of-hooks
    useState<T>(key === 'selectedPage' ? ('notes.md' as T) : initial),
}))
vi.mock('@s4wave/app/sobject/space-replay.js', () => ({
  useSaveHold: () => null,
}))
vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: vi.fn(), info: vi.fn() },
}))

// handleOf returns a handle whose hold returns the handle. release counts
// released holds.
function handleOf(fields: Record<string, unknown>): FSHandle {
  const handle = { ...fields, released: false, release: vi.fn() }
  return { ...handle, hold: () => handle } as unknown as FSHandle
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('documentation page save', () => {
  it('decides a save sent before the viewer closed, once, holding its handles until then', async () => {
    for (const outcome of ['written', 'refused'] as const) {
      // Send a save the Space holds while it opens.
      let decide: (result: Promise<void>) => void = () => {}
      const writeAt = vi.fn(
        () => new Promise<void>((resolve) => (decide = resolve)),
      )
      const fileHandle = handleOf({
        writeAt,
        truncate: vi.fn().mockResolvedValue(undefined),
        sync: vi.fn().mockResolvedValue(undefined),
      })
      file.handle = fileHandle
      const root = handleOf({})
      const rootHandle = { value: root } as unknown as Resource<FSHandle>
      const view = renderHook(() =>
        useDocPage({} as StateNamespace, rootHandle),
      )
      await act(() => view.result.current.toggleEdit())
      act(() => view.result.current.setEditContent('held'))
      let saved: Promise<void> = Promise.resolve()
      act(() => {
        saved = view.result.current.toggleEdit()
      })
      await waitFor(() => expect(writeAt).toHaveBeenCalledTimes(1))

      // A second press while the save runs sends no second write.
      await view.result.current.toggleEdit()
      expect(writeAt).toHaveBeenCalledTimes(1)

      // Close the viewer: the save runs on, and its handles are released
      // only once the Space decides it.
      view.unmount()
      const releases = [fileHandle.release, root.release] as ReturnType<
        typeof vi.fn
      >[]
      for (const release of releases) expect(release).not.toHaveBeenCalled()
      decide(
        outcome === 'written'
          ? Promise.resolve()
          : Promise.reject(
              new Error(
                'the Space is catching up and is read only until it does',
              ),
            ),
      )
      await saved
      for (const release of releases) expect(release).toHaveBeenCalledTimes(1)

      // A refusal the closed viewer missed shows in a toast. A landed save
      // shows nothing.
      if (outcome === 'written') {
        expect(toast.error).not.toHaveBeenCalled()
      } else {
        expect(toast.error).toHaveBeenCalledWith(
          'Your edits to notes.md were not saved.',
        )
      }
      expect(writeAt).toHaveBeenCalledTimes(1)
      cleanup()
      vi.clearAllMocks()
    }
  })
})
