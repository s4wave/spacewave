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

// handles.file is the file handle the mocked hook returns; replacing it makes
// the viewer read the file again, as a new handle does.
const handles = vi.hoisted(() => ({
  file: {
    readAt: (() => {}) as (...args: unknown[]) => unknown,
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

// readsText makes the file handle read the text, whole unless eof is false.
function readsText(text: string, eof = true) {
  handles.file = {
    readAt: vi
      .fn()
      .mockResolvedValue({ data: new TextEncoder().encode(text), eof }),
    getInfo: () => ({ mode: 0o644 }),
  }
}

// diskHolds makes the same file handle read text from now on, as the disk
// does after a commit no watch has reported yet.
function diskHolds(text: string) {
  handles.file.readAt = vi
    .fn()
    .mockResolvedValue({ data: new TextEncoder().encode(text), eof: true })
}

// diskLost makes the same file handle fail its reads as a removed file
// does: Go's fs.ErrNotExist arrives as its message.
function diskLost() {
  handles.file.readAt = vi
    .fn()
    .mockRejectedValue(new Error('file does not exist'))
}

// readFails makes the next read of the file fail.
function readFails(message: string) {
  handles.file = {
    readAt: vi.fn().mockRejectedValue(new Error(message)),
    getInfo: () => ({ mode: 0o644 }),
  }
}

function rootResource(uploadTree = vi.fn().mockResolvedValue(undefined)) {
  return {
    value: { uploadTree } as unknown as FSHandle,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

// typeInto replaces the draft's text.
function typeInto(text: string) {
  fireEvent.change(screen.getByRole('textbox', { name: 'File contents' }), {
    target: { value: text },
  })
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('UnixFS text editing', () => {
  it('shows a landed save, keeps the draft through a failed save, and never resaves on reconnect', async () => {
    // The first save lands; the second fails.
    readsText('original')
    const upload = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('offline'))
    const root = rootResource(upload)
    const view = render(
      <UnixFSTextFileViewer rootHandle={root} path="/entry.ts" />,
    )

    // A landed save shows its text.
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('accepted')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('accepted')
    diskHolds('accepted')

    // A failed save keeps the draft; Discard edits shows the landed text.
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    typeInto('unsaved')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect((await screen.findByRole('alert')).textContent).toContain('offline')
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'unsaved',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Discard edits' }))
    expect(screen.getByText('accepted')).toBeDefined()

    // A reconnected root saves nothing on its own.
    const reconnected = rootResource()
    view.rerender(
      <UnixFSTextFileViewer rootHandle={reconnected} path="/entry.ts" />,
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit' })).toBeDefined(),
    )
    expect(reconnected.value.uploadTree).not.toHaveBeenCalled()
    expect(upload).toHaveBeenCalledTimes(2)
  })

  it('offers no Edit on a truncated preview', async () => {
    // A read cut at the limit is a preview.
    readsText('partial content', false)
    render(
      <UnixFSTextFileViewer rootHandle={rootResource()} path="/large.txt" />,
    )

    // It offers no Edit and says why.
    await screen.findByText('partial content')
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
    expect(
      screen.getByText('This preview is too large to edit here.'),
    ).toBeDefined()
  })

  it('offers no Edit on a read-only file and says why', async () => {
    // A commit's file is read-only.
    readsText('committed')
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource()}
        path="/README.md"
        readOnly="Read-only: a file in a commit"
      />,
    )

    // It offers no Edit and says why.
    await screen.findByText('committed')
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
    expect(screen.getByText('Read-only: a file in a commit')).toBeDefined()
  })

  // A failed re-read used to replace the viewer with its failure, hiding an
  // open draft.
  it('keeps a dirty draft in the same textarea when a re-read fails', async () => {
    // Open a dirty draft.
    readsText('original')
    const root = rootResource()
    const view = render(
      <UnixFSTextFileViewer rootHandle={root} path="/notes.md" />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('my edits')
    const textarea = screen.getByRole('textbox', { name: 'File contents' })

    // The file is read again and the read fails.
    readFails('stream closed')
    view.rerender(<UnixFSTextFileViewer rootHandle={root} path="/notes.md" />)

    // One notice with Retry; the same textarea keeps the draft.
    const alerts = await screen.findAllByRole('alert')
    expect(alerts).toHaveLength(1)
    expect(alerts[0].textContent).toContain('stream closed')
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(1)
    expect(screen.getByRole('textbox', { name: 'File contents' })).toBe(
      textarea,
    )
    expect((textarea as HTMLTextAreaElement).value).toBe('my edits')
  })

  it('never offers a plain Save over another writer', async () => {
    // Open a dirty draft.
    readsText('original')
    const upload = vi.fn().mockResolvedValue(undefined)
    const root = rootResource(upload)
    const view = render(
      <UnixFSTextFileViewer rootHandle={root} path="/notes.md" />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')

    // Another writer saves; the file is read again.
    readsText('theirs')
    view.rerender(<UnixFSTextFileViewer rootHandle={root} path="/notes.md" />)

    // Plain Save leaves and the draft stays.
    await screen.findByText(
      'Another writer saved this file after you began editing.',
    )
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'mine',
    )

    // Show theirs shows their text, and back.
    fireEvent.click(screen.getByRole('button', { name: 'Show theirs' }))
    expect(screen.getByLabelText('Their version').textContent).toBe('theirs')
    fireEvent.click(screen.getByRole('button', { name: 'Back to my edits' }))

    // Save over theirs writes the draft.
    fireEvent.click(screen.getByRole('button', { name: 'Save over theirs' }))
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
  })

  it('stops a plain Save that finds another writer saved', async () => {
    // Open a dirty draft; another writer commits and no read has run yet.
    readsText('original')
    const upload = vi.fn().mockResolvedValue(undefined)
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    diskHolds('theirs')

    // The read at the press finds their version: nothing is written, and
    // the draft shows the change with Save over theirs.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByRole('button', { name: 'Save over theirs' })
    expect(upload).not.toHaveBeenCalled()
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'mine',
    )
  })

  it('recreates a file removed before a plain Save', async () => {
    // Open a dirty draft; the file is removed elsewhere.
    readsText('original')
    const upload = vi.fn().mockResolvedValue(undefined)
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    diskLost()

    // The read at the press finds nothing, so the draft is written.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('mine')
    expect(upload).toHaveBeenCalledTimes(1)
  })

  it('keeps the draft with Retry when the read at Save fails', async () => {
    // Open a dirty draft; the next read fails for another reason.
    readsText('original')
    const upload = vi.fn().mockResolvedValue(undefined)
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    handles.file.readAt = vi.fn().mockRejectedValue(new Error('offline'))

    // The save fails unwritten and the draft stays.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect((await screen.findByRole('alert')).textContent).toContain('offline')
    expect(upload).not.toHaveBeenCalled()
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'mine',
    )
  })

  it('offers no Edit on a file that is not UTF-8', async () => {
    // A whole file with an invalid byte is a preview.
    handles.file = {
      readAt: vi
        .fn()
        .mockResolvedValue({ data: new Uint8Array([0x61, 0xff]), eof: true }),
      getInfo: () => ({ mode: 0o644 }),
    }
    render(<UnixFSTextFileViewer rootHandle={rootResource()} path="/a.bin" />)

    // It offers no Edit and says why.
    await screen.findByText(
      'This file is not UTF-8 text, so it cannot be edited here.',
    )
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
  })
})
