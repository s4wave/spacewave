import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { UnixFSTextFileViewer } from './UnixFSTextFileViewer.js'

const handles = vi.hoisted(() => ({
  file: {
    readAt: vi.fn(),
    getInfo: () => ({ mode: 0o644 }),
  },
}))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  unixFSHandleTextContentMaxBytes: 512 * 1024,
  useUnixFSHandle: () => ({
    value: handles.file,
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

function rootResource(uploadTree = vi.fn().mockResolvedValue(undefined)) {
  return {
    value: { uploadTree } as unknown as FSHandle,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('UnixFS text editing', () => {
  it('keeps the last accepted text through a failed save and never resaves on reconnect', async () => {
    handles.file.readAt.mockResolvedValue({
      data: new TextEncoder().encode('original'),
      eof: true,
    })
    const upload = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('offline'))
    const root = rootResource(upload)
    const view = render(
      <UnixFSTextFileViewer rootHandle={root} path="/entry.ts" />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit file' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'File contents' }), {
      target: { value: 'accepted' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save file' }))
    await screen.findByText('File saved')

    fireEvent.change(screen.getByRole('textbox', { name: 'File contents' }), {
      target: { value: 'unsaved' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save file' }))
    expect((await screen.findByRole('alert')).textContent).toContain('offline')
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'unsaved',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Cancel edits' }))
    expect(screen.getByText('accepted')).toBeDefined()

    const reconnected = rootResource()
    view.rerender(
      <UnixFSTextFileViewer rootHandle={reconnected} path="/entry.ts" />,
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit file' })).toBeDefined(),
    )
    expect(reconnected.value.uploadTree).not.toHaveBeenCalled()
    expect(upload).toHaveBeenCalledTimes(2)
  })

  it('keeps a truncated preview read-only', async () => {
    handles.file.readAt.mockResolvedValue({
      data: new TextEncoder().encode('partial content'),
      eof: false,
    })
    render(
      <UnixFSTextFileViewer rootHandle={rootResource()} path="/large.txt" />,
    )
    const edit = await screen.findByRole('button', { name: 'Edit file' })
    expect((edit as HTMLButtonElement).disabled).toBe(true)
    expect(
      screen.getByText('This preview is too large to edit here.'),
    ).toBeDefined()
  })
})
