import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'

const root = document.createElement('div')
const registerUpdateListener = vi.fn(() => vi.fn())

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

  it('submits a changed export while suppressing the same failed body', async () => {
    let exported = 'first'
    const onSave = vi
      .fn<(content: string) => Promise<void>>()
      .mockRejectedValueOnce(new Error('disk full'))
      .mockResolvedValueOnce()

    render(
      <SavePlugin
        savedContent="original"
        exportString={() => exported}
        onSave={onSave}
      />,
    )

    fireEvent.blur(root)
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    fireEvent.blur(root)
    expect(onSave).toHaveBeenCalledOnce()

    exported = 'second'
    fireEvent.blur(root)
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2))
    expect(onSave).toHaveBeenNthCalledWith(2, 'second')
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
