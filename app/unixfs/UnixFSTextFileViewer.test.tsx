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
// the viewer read the file again, as a new handle does. handles.disk is what a
// new lookup under the root reads: the committed file, or a failure, at
// handles.diskVersion when set and otherwise at its text's version.
const handles = vi.hoisted(() => ({
  file: {
    readAt: (() => {}) as (...args: unknown[]) => unknown,
  },
  disk: 'original' as string | Error,
  diskVersion: null as Uint8Array | null,
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
  }
}

// diskHolds makes a new lookup under the root read text, while the file's
// own handle still reads what it did, as after a commit through the root.
function diskHolds(text: string | Error) {
  handles.disk = text
}

// uploadedText returns the bytes an upload call sent for its one file.
async function uploadedBytes(upload: ReturnType<typeof vi.fn>, call: number) {
  const entries = upload.mock.calls[call][0] as { stream: ReadableStream }[]
  return new Uint8Array(await new Response(entries[0].stream).arrayBuffer())
}

// writtenAt is the result of an upload the store committed, at the version
// of text.
function writtenAt(text: string) {
  return {
    kind: 'written',
    bytesWritten: 1n,
    filesWritten: 1n,
    directoriesWritten: 0n,
    versions: [{ path: 'notes.md', version: versionOf(text) }],
  }
}

// expectOf returns the expectation an upload call sent for its one file.
function expectOf(upload: ReturnType<typeof vi.fn>, call: number) {
  const entries = upload.mock.calls[call][0] as { expect: unknown }[]
  return entries[0].expect
}

// entryOf returns the one file entry an upload call sent.
function entryOf(upload: ReturnType<typeof vi.fn>, call: number) {
  return (upload.mock.calls[call][0] as Record<string, unknown>[])[0]
}

// readFails makes the next read of the file fail.
function readFails(message: string) {
  handles.file = {
    readAt: vi.fn().mockRejectedValue(new Error(message)),
  }
}

// lookupPath opens a handle on the committed root that reads handles.disk.
function lookupPath() {
  const disk = handles.disk
  const version = handles.diskVersion
  const readAt = () =>
    disk instanceof Error
      ? Promise.reject(disk)
      : Promise.resolve({
          ...readOf(disk),
          version: version ?? versionOf(disk),
        })
  return Promise.resolve({ handle: { readAt, release: () => {} } })
}

function rootResource(
  uploadTree = vi.fn().mockResolvedValue(writtenAt('saved')),
) {
  return {
    value: { uploadTree, lookupPath } as unknown as FSHandle,
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
  handles.disk = 'original'
  handles.diskVersion = null
  cleanup()
  vi.clearAllMocks()
})

describe('UnixFS text editing', () => {
  it('shows a landed save, keeps the draft through a failed save, and never resaves on reconnect', async () => {
    // The first save lands; the second fails.
    readsText('original')
    const upload = vi
      .fn()
      .mockResolvedValueOnce(writtenAt('accepted'))
      .mockRejectedValueOnce(new Error('offline'))
    const root = rootResource(upload)
    const view = render(
      <UnixFSTextFileViewer rootHandle={root} path="/entry.ts" />,
    )

    // A landed save expects the version read, asks for the version it writes
    // and sends no mode, and shows its text.
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('accepted')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('accepted')
    expect(expectOf(upload, 0)).toEqual({
      kind: 'version',
      version: versionOf('original'),
    })
    expect(entryOf(upload, 0)).toMatchObject({ wantVersion: true })
    expect(entryOf(upload, 0)).not.toHaveProperty('mode')

    // The next Save expects the version the save wrote; it fails and keeps
    // the draft, and Discard edits shows the landed text.
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
    const upload = vi.fn().mockResolvedValue(writtenAt('mine'))
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
      .mockResolvedValueOnce(writtenAt('mine'))
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')

    // The same press writes the draft again, only while nothing holds the
    // path.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('mine')
    expect(upload).toHaveBeenCalledTimes(2)
    expect(expectOf(upload, 1)).toEqual({ kind: 'missing' })
  })

  it('offers Save over theirs and Read again when the read after a refusal fails', async () => {
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
    diskHolds(new Error('offline'))

    // The save stops unwritten; the draft stays with Save over theirs, and
    // plain Save is gone.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('Could not read their version: offline.')
    expect(
      screen.getByRole('button', { name: 'Save over theirs' }),
    ).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'mine',
    )

    // Read again brings their version.
    diskHolds('theirs')
    fireEvent.click(screen.getByRole('button', { name: 'Read again' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Show theirs' }))
    expect(screen.getByLabelText('Their version').textContent).toBe('theirs')
  })

  it('saves next from the version a save wrote, past a read of the one before', async () => {
    // Open a dirty draft; the save lands.
    readsText('original')
    const upload = vi.fn().mockResolvedValue(writtenAt('mine'))
    const root = rootResource(upload)
    const view = render(
      <UnixFSTextFileViewer rootHandle={root} path="/notes.md" />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('mine')

    // The file's own handle still reads the version the save replaced; the
    // saved text stays.
    readsText('original')
    view.rerender(<UnixFSTextFileViewer rootHandle={root} path="/notes.md" />)
    await waitFor(() => expect(handles.file.readAt).toHaveBeenCalled())
    expect(screen.getByText('mine')).toBeDefined()

    // The next Save expects the version the save wrote, with no read between.
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    typeInto('mine again')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(2))
    expect(expectOf(upload, 1)).toEqual({
      kind: 'version',
      version: versionOf('mine'),
    })
  })

  it('says the text did not change when another writer changed only the file', async () => {
    // Open a dirty draft; another writer changes the file's permissions, so
    // its version moves while its text stays.
    readsText('original')
    const upload = vi
      .fn()
      .mockResolvedValueOnce({
        kind: 'conflict',
        path: 'notes.md',
        version: new Uint8Array([7]),
      })
      .mockResolvedValueOnce(writtenAt('mine'))
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    handles.diskVersion = new Uint8Array([7])

    // The refusal reads back the same text: plain Save stays, and the notice
    // says the text did not change.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText(/changed this file, but not its text/)
    expect(
      screen.queryByRole('button', { name: 'Save over theirs' }),
    ).toBeNull()

    // Save again expects the version read back.
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(2))
    expect(expectOf(upload, 1)).toEqual({
      kind: 'version',
      version: new Uint8Array([7]),
    })
  })

  it('keeps the draft when a landed save returns no version', async () => {
    // The store commits the save but names no version it wrote.
    readsText('original')
    const upload = vi
      .fn()
      .mockResolvedValue({ ...writtenAt('mine'), versions: [] })
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.md"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    // The save fails with the reason, and the draft stays to save again.
    expect((await screen.findByRole('alert')).textContent).toContain(
      'the save returned no version',
    )
    expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(
      'mine',
    )
  })

  it('shows a save over theirs past their version read back before it', async () => {
    // A plain save is refused; the read back of their version hangs.
    readsText('original')
    const upload = vi
      .fn()
      .mockResolvedValueOnce({
        kind: 'conflict',
        path: 'notes.md',
        version: versionOf('theirs'),
      })
      .mockResolvedValueOnce(writtenAt('mine'))
    let backSignal: AbortSignal | undefined
    let land: (value: unknown) => void = () => {}
    const root = rootResource(upload)
    root.value = {
      uploadTree: upload,
      lookupPath: (_path: unknown, signal: AbortSignal) => {
        backSignal = signal
        return Promise.resolve({
          handle: {
            readAt: () => new Promise((resolve) => (land = resolve)),
            release: () => {},
          },
        })
      },
    } as unknown as FSHandle
    render(<UnixFSTextFileViewer rootHandle={root} path="/notes.md" />)
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('mine')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByText('Reading their version.')

    // Save over theirs lands while the read runs; the read is aborted.
    fireEvent.click(screen.getByRole('button', { name: 'Save over theirs' }))
    await screen.findByText('mine')
    expect(backSignal?.aborted).toBe(true)

    // Their version arriving late leaves the saved text shown.
    land(readOf('theirs'))
    await Promise.resolve()
    expect(screen.queryByText('theirs')).toBeNull()
    expect(screen.getByText('mine')).toBeDefined()
  })

  it('offers no Edit on a file that is not UTF-8', async () => {
    // A whole file with an invalid byte is a preview.
    handles.file = {
      readAt: vi.fn().mockResolvedValue({
        data: new Uint8Array([0x61, 0xff]),
        eof: true,
        version: versionOf('a\uFFFD'),
      }),
    }
    render(<UnixFSTextFileViewer rootHandle={rootResource()} path="/a.bin" />)

    // It offers no Edit and says why.
    await screen.findByText(
      'This file is not UTF-8 text, so it cannot be edited here.',
    )
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
  })

  it('saves an edit to a CRLF file with a byte order mark as only that edit', async () => {
    // A file with a byte order mark and CRLF lines, edited by one line.
    const file = '\uFEFFfirst\r\nsecond\r\n'
    const saved = file + 'third\r\n'
    readsText(file)
    const upload = vi.fn().mockResolvedValue(writtenAt(saved))
    render(
      <UnixFSTextFileViewer
        rootHandle={rootResource(upload)}
        path="/notes.txt"
      />,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    const box = screen.getByRole('textbox', { name: 'File contents' })
    typeInto((box as HTMLTextAreaElement).value + 'third\n')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    // The upload keeps the mark and the CRLF lines, and the next draft
    // starts from the saved text.
    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
    expect(await uploadedBytes(upload, 0)).toEqual(
      new TextEncoder().encode(saved),
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    typeInto('\uFEFFfirst\n')
    await waitFor(() =>
      expect(
        screen
          .getByRole('button', { name: 'Save' })
          .getAttribute('aria-disabled'),
      ).toBeNull(),
    )
  })
})
