import { useState } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import { DocumentationEditor } from './DocumentationEditor.js'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function resource<T>(value: T): Resource<T> {
  return { value, loading: false, error: null, retry: vi.fn() }
}

function editor(handle: FSHandle, setEditing = vi.fn()) {
  return (
    <DocumentationEditor
      page="guide.md"
      handle={resource(handle)}
      text={resource('# guide')}
      editing
      setEditing={setEditing}
    />
  )
}

function PersistedEditor({
  handle,
  text,
}: {
  handle: FSHandle
  text: Resource<string>
}) {
  const [editing, setEditing] = useState(true)
  return (
    <DocumentationEditor
      page="guide.md"
      handle={resource(handle)}
      text={text}
      editing={editing}
      setEditing={setEditing}
    />
  )
}

afterEach(cleanup)

describe('DocumentationEditor', () => {
  it('retains a rejected draft and offers retry until save succeeds', async () => {
    const handle = {
      writeAt: vi
        .fn()
        .mockRejectedValueOnce(new Error('disk full'))
        .mockResolvedValueOnce(7n),
      truncate: vi.fn().mockResolvedValue(undefined),
    } as unknown as FSHandle
    const setEditing = vi.fn()
    render(editor(handle, setEditing))

    fireEvent.change(screen.getByLabelText('Documentation page content'), {
      target: { value: 'changed' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect((await screen.findByRole('alert')).textContent).toContain(
      'Could not save: disk full',
    )
    expect(
      (
        screen.getByLabelText(
          'Documentation page content',
        ) as HTMLTextAreaElement
      ).value,
    ).toBe('changed')
    expect(setEditing).not.toHaveBeenCalledWith(false)

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(setEditing).toHaveBeenCalledWith(false))
    expect(handle.truncate).toHaveBeenCalledWith(7n, expect.any(AbortSignal))
  })

  it('retains the draft when truncation fails after the write', async () => {
    const handle = {
      writeAt: vi.fn().mockResolvedValue(7n),
      truncate: vi.fn().mockRejectedValue(new Error('truncate failed')),
    } as unknown as FSHandle
    render(editor(handle))
    fireEvent.change(screen.getByLabelText('Documentation page content'), {
      target: { value: 'changed' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect((await screen.findByRole('alert')).textContent).toContain(
      'truncate failed',
    )
    expect(
      (
        screen.getByLabelText(
          'Documentation page content',
        ) as HTMLTextAreaElement
      ).value,
    ).toBe('changed')
  })

  it('admits only one save while an earlier save is pending', async () => {
    const pending = deferred<bigint>()
    const handle = {
      writeAt: vi.fn().mockReturnValue(pending.promise),
      truncate: vi.fn().mockResolvedValue(undefined),
    } as unknown as FSHandle
    render(editor(handle))

    const save = screen.getByRole('button', { name: 'Save' })
    fireEvent.click(save)
    fireEvent.click(save)
    expect(handle.writeAt).toHaveBeenCalledOnce()

    pending.resolve(7n)
    await waitFor(() => expect(handle.truncate).toHaveBeenCalledOnce())
  })

  it('aborts a pending save on cancel and keeps stale completion inert', async () => {
    const pending = deferred<bigint>()
    let signal: AbortSignal | undefined
    const handle = {
      writeAt: vi.fn().mockImplementation((_offset, _data, value) => {
        signal = value
        return pending.promise
      }),
      truncate: vi.fn().mockResolvedValue(undefined),
    } as unknown as FSHandle
    const setEditing = vi.fn()
    render(editor(handle, setEditing))

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(signal?.aborted).toBe(true)
    expect(setEditing).toHaveBeenCalledWith(false)

    pending.resolve(7n)
    await Promise.resolve()
    expect(handle.truncate).toHaveBeenCalledWith(7n, signal)
    expect(setEditing).toHaveBeenCalledTimes(1)
  })

  it('ignores a save completion after navigation unmounts the editor', async () => {
    const pending = deferred<bigint>()
    const handle = {
      writeAt: vi.fn().mockReturnValue(pending.promise),
      truncate: vi.fn().mockResolvedValue(undefined),
    } as unknown as FSHandle
    const setEditing = vi.fn()
    const view = render(editor(handle, setEditing))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    view.unmount()

    pending.resolve(7n)
    await Promise.resolve()
    await Promise.resolve()
    expect(setEditing).not.toHaveBeenCalled()
  })

  it('saves hydrated text unchanged and returns a persisted edit to preview', async () => {
    const handle = {
      writeAt: vi.fn().mockResolvedValue(12n),
      truncate: vi.fn().mockResolvedValue(undefined),
    } as unknown as FSHandle
    const loadingText: Resource<string> = {
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    }
    const view = render(<PersistedEditor handle={handle} text={loadingText} />)
    view.rerender(
      <PersistedEditor handle={handle} text={resource('loaded draft')} />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit' })).toBeTruthy(),
    )
    expect(handle.writeAt).toHaveBeenCalledWith(
      0n,
      new TextEncoder().encode('loaded draft'),
      expect.any(AbortSignal),
    )
    expect(handle.truncate).toHaveBeenCalledWith(12n, expect.any(AbortSignal))
    expect(vi.mocked(handle.writeAt).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(handle.truncate).mock.invocationCallOrder[0],
    )
    expect(screen.queryByLabelText('Documentation page content')).toBeNull()
  })

  it('does not overwrite a user draft when page text arrives late', async () => {
    const handle = {
      writeAt: vi.fn().mockResolvedValue(10n),
      truncate: vi.fn().mockResolvedValue(undefined),
    } as unknown as FSHandle
    const emptyText: Resource<string> = {
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    }
    const view = render(<PersistedEditor handle={handle} text={emptyText} />)
    fireEvent.change(screen.getByLabelText('Documentation page content'), {
      target: { value: 'user draft' },
    })

    view.rerender(
      <PersistedEditor handle={handle} text={resource('loaded draft')} />,
    )

    expect(
      (
        screen.getByLabelText(
          'Documentation page content',
        ) as HTMLTextAreaElement
      ).value,
    ).toBe('user draft')

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(handle.writeAt).toHaveBeenCalledOnce())
    expect(handle.writeAt).toHaveBeenCalledWith(
      0n,
      new TextEncoder().encode('user draft'),
      expect.any(AbortSignal),
    )
  })
})
