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

// versionOf is the version a read of text reports: each text its own.
function versionOf(text: string): Uint8Array {
  return new TextEncoder().encode('v:' + text)
}

// readOf is the file handle's read of text, whole unless eof is false.
function readOf(text: string, eof = true) {
  const data = new TextEncoder().encode(text)
  return { data, eof, version: versionOf(text) }
}

// readsText makes the file handle read the text, whole unless eof is false.
function readsText(text: string, eof = true) {
  handles.file = {
    readAt: vi.fn().mockResolvedValue(readOf(text, eof)),
    getInfo: () => ({ mode: 0o644 }),
  }
}

// diskHolds makes the same file handle read text from now on, as the disk
// does after a commit no watch has reported yet.
function diskHolds(text: string) {
  handles.file.readAt = vi.fn().mockResolvedValue(readOf(text))
}

// written is the result of an upload the store committed.
const written = {
  kind: 'written',
  bytesWritten: 1n,
  filesWritten: 1n,
  directoriesWritten: 0n,
}

// expectOf returns the expectation an upload call sent for its one file.
function expectOf(upload: ReturnType<typeof vi.fn>, call: number) {
  const entries = upload.mock.calls[call][0] as { expect: unknown }[]
  return entries[0].expect
}

// readFails makes the next read of the file fail.
function readFails(message: string) {
  handles.file = {
    readAt: vi.fn().mockRejectedValue(new Error(message)),
    getInfo: () => ({ mode: 0o644 }),
  }
}

function rootResource(uploadTree = vi.fn().mockResolvedValue(written)) {
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
      .mockResolvedValueOnce(written)
      .mockRejectedValueOnce(new Error('offline'))
    const root = rootResource(upload)
    const view = render(
      <UnixFSTextFileViewer rootHandle={root} path="/entry.ts" />,
    )

    // A landed save expects the version read and shows the text read back.
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('accepted')
    diskHolds('accepted')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('accepted')
    expect(expectOf(upload, 0)).toEqual({
      kind: 'version',
      version: versionOf('original'),
    })

    // The next Save expects the version the save read back; it fails and
    // keeps the draft, and Discard edits shows the landed text.
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    typeInto('unsaved')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(expectOf(upload, 1)).toEqual({
      kind: 'version',
      version: versionOf('accepted'),
    })
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
    const upload = vi.fn().mockResolvedValue(written)
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

    // Save over theirs writes the draft whatever the file holds.
    fireEvent.click(screen.getByRole('button', { name: 'Save over theirs' }))
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
    expect(expectOf(upload, 0)).toEqual({ kind: 'overwrite' })
  })

  it('shows the change when the store refuses a plain Save', async () => {
    // Open a dirty draft; another writer commits and no read has run yet.
    readsText('original')
    const upload = vi.fn().mockResolvedValue({
      kind: 'conflict',
      path: 'notes.md',
      version: versionOf('theirs'),
    })
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    diskHolds('theirs')

    // The store refuses the write; the read back shows their version, with
    // Save over theirs, and the draft stays.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByRole('button', { name: 'Save over theirs' })
    expect(upload).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Show theirs' }))
    expect(screen.getByLabelText('Their version').textContent).toBe('theirs')
    fireEvent.click(screen.getByRole('button', { name: 'Back to my edits' }))
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'mine',
    )
  })

  it('recreates a file another writer removed before a plain Save', async () => {
    // Open a dirty draft; the file is removed elsewhere, so the store
    // refuses the version with no file at the path.
    readsText('original')
    const upload = vi
      .fn()
      .mockResolvedValueOnce({ kind: 'conflict', path: 'notes.md' })
      .mockResolvedValueOnce(written)
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    diskHolds('mine')

    // The same press writes the draft again, only while nothing holds the
    // path.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('mine')
    expect(upload).toHaveBeenCalledTimes(2)
    expect(expectOf(upload, 1)).toEqual({ kind: 'missing' })
  })

  it('keeps the draft with Retry when the read after a refusal fails', async () => {
    // Open a dirty draft; the store refuses the save and the read back
    // fails.
    readsText('original')
    const upload = vi.fn().mockResolvedValue({
      kind: 'conflict',
      path: 'notes.md',
      version: versionOf('theirs'),
    })
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    handles.file.readAt = vi.fn().mockRejectedValue(new Error('offline'))

    // The save stops unwritten and the draft stays.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect((await screen.findByRole('alert')).textContent).toContain('offline')
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'mine',
    )
  })

  it('offers no Edit on a file that is not UTF-8', async () => {
    // A whole file with an invalid byte is a preview.
    handles.file = {
      readAt: vi.fn().mockResolvedValue({
        data: new Uint8Array([0x61, 0xff]),
        eof: true,
        version: versionOf('a\uFFFD'),
      }),
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
