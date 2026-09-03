import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'

const root = document.createElement('div')
interface EditorUpdate {
  editorState: object
  prevEditorState: object
}
let updateListeners: Array<(update: EditorUpdate) => void> = []
const registerUpdateListener = vi.fn(
  (listener: (update: EditorUpdate) => void) => {
    updateListeners.push(listener)
    return () => {
      updateListeners = updateListeners.filter(
        (registered) => registered !== listener,
      )
    }
  },
)

// Simulates a keystroke: every editor state change reaches the plugin through
// registerUpdateListener with distinct editor state objects.
function fireEditorUpdate() {
  const prevEditorState = {}
  const editorState = {}
  for (const listener of updateListeners) {
    listener({ editorState, prevEditorState })
  }
}

vi.mock('@lexical/react/LexicalComposerContext', () => ({
  useLexicalComposerContext: () => [
    {
      getEditorState: () => ({ read: (callback: () => void) => callback() }),
      getRootElement: () => root,
      registerUpdateListener,
    },
  ],
}))

import SavePlugin from './SavePlugin.js'

describe('SavePlugin', () => {
  afterEach(() => {
    cleanup()
    updateListeners = []
    vi.clearAllMocks()
  })

  it('accepts an export only after the save resolves', async () => {
    let resolveSave: (() => void) | undefined
    const onSave = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve
        }),
    )

    render(
      <SavePlugin
        savedContent="original"
        exportString={() => 'draft'}
        onSave={onSave}
      />,
    )

    fireEvent.blur(root)
    fireEvent.blur(root)
    expect(onSave).toHaveBeenCalledOnce()

    resolveSave?.()
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())

    fireEvent.blur(root)
    expect(onSave).toHaveBeenCalledOnce()
  })

  it('refreshes the accepted baseline when saved content changes', () => {
    const onSave = vi.fn()
    let exported = 'draft'
    const view = render(
      <SavePlugin
        savedContent="original"
        exportString={() => exported}
        onSave={onSave}
      />,
    )

    exported = 'accepted'
    view.rerender(
      <SavePlugin
        savedContent="accepted"
        exportString={() => exported}
        onSave={onSave}
      />,
    )
    fireEvent.blur(root)
    expect(onSave).not.toHaveBeenCalled()
  })

  it('suppresses automatic re-save of edited drafts until the retry clears the failure', async () => {
    let exported = 'first'
    const onDraftChange = vi.fn()
    const onSave = vi
      .fn<(content: string) => Promise<void>>()
      .mockRejectedValueOnce(new Error('disk full'))

    const view = render(
      <SavePlugin
        savedContent="original"
        exportString={() => exported}
        onSave={onSave}
        onDraftChange={onDraftChange}
      />,
    )

    fireEvent.blur(root)
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    // The rejected body stays the published draft instead of vanishing.
    await waitFor(() => expect(onDraftChange).toHaveBeenCalledWith('first'))

    // A keystroke goes through registerUpdateListener -> markDirty, which
    // refreshes the failure marker to the newest export and keeps carrying
    // the draft text forward.
    exported = 'second'
    fireEditorUpdate()
    expect(onDraftChange).toHaveBeenLastCalledWith('second')

    // The next export sees content equal to the refreshed failure marker and
    // skips, so the failed body never retries automatically.
    fireEvent.blur(root)
    await Promise.resolve()
    expect(onSave).toHaveBeenCalledOnce()
    expect(onDraftChange).toHaveBeenLastCalledWith('second')

    // The explicit retry path accepts the draft as saved and clears the
    // failure, so later exports submit again.
    exported = 'third'
    view.rerender(
      <SavePlugin
        savedContent="second"
        exportString={() => exported}
        onSave={onSave}
        onDraftChange={onDraftChange}
      />,
    )
    fireEvent.blur(root)
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2))
    expect(onSave).toHaveBeenNthCalledWith(2, 'third')
  })
  it('publishes the newest draft at edit time while a save is in flight', async () => {
    let exported = 'A'
    let resolveSave: (() => void) | undefined
    const onDraftChange = vi.fn()
    const onSave = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve
        }),
    )

    render(
      <SavePlugin
        savedContent="original"
        exportString={() => exported}
        onSave={onSave}
        onDraftChange={onDraftChange}
      />,
    )

    // The write of A goes out and stays in flight.
    fireEvent.blur(root)
    expect(onSave).toHaveBeenCalledWith('A')
    await waitFor(() => expect(onDraftChange).toHaveBeenCalledWith('A'))

    // A keystroke produces text B while the write of A is still in flight.
    exported = 'B'
    fireEditorUpdate()

    // The plugin must publish B at edit time so the save pipeline can treat
    // the in-flight write of A as superseded instead of settling 'Saved' for
    // text the user has already replaced.
    expect(onDraftChange).toHaveBeenLastCalledWith('B')

    // Completing the stale write of A must not settle the pipeline back onto
    // A: B remains the published draft and is the next thing saved.
    resolveSave?.()
    await Promise.resolve()
    fireEvent.blur(root)
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2))
    expect(onSave).toHaveBeenNthCalledWith(2, 'B')
  })

  it('submits Y after pending X rejects without suppressing Y', async () => {
    let exported = 'X'
    let rejectX: ((error: Error) => void) | undefined
    const onSave = vi
      .fn<(content: string) => Promise<void>>()
      .mockImplementationOnce(
        () =>
          new Promise<void>((_resolve, reject) => {
            rejectX = reject
          }),
      )
      .mockResolvedValueOnce()

    render(
      <SavePlugin
        savedContent="original"
        exportString={() => exported}
        onSave={onSave}
      />,
    )

    fireEvent.blur(root)
    expect(onSave).toHaveBeenCalledWith('X')
    exported = 'Y'
    rejectX?.(new Error('X failed'))
    await Promise.resolve()
    await Promise.resolve()

    fireEvent.blur(root)
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2))
    expect(onSave).toHaveBeenNthCalledWith(2, 'Y')
  })
})
