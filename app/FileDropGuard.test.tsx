import { cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  APP_DRAG_MIME,
  hasNativeFileDrag,
} from '@s4wave/web/dnd/app-drag.js'

import { FileDropGuard } from './FileDropGuard.js'

function createDragEvent(
  type: 'dragover' | 'drop',
  dataTransfer: DataTransfer,
) {
  const event = new Event(type, { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'dataTransfer', { value: dataTransfer })
  return event as DragEvent
}

describe('FileDropGuard', () => {
  afterEach(cleanup)

  it('prevents native file drops from navigating the document', () => {
    render(<FileDropGuard />)
    const dataTransfer = {
      items: [{ kind: 'file' }],
      types: ['Files'],
      dropEffect: 'copy',
    } as unknown as DataTransfer

    const dragOver = createDragEvent('dragover', dataTransfer)
    expect(document.dispatchEvent(dragOver)).toBe(false)
    expect(dragOver.defaultPrevented).toBe(true)
    expect(dataTransfer.dropEffect).toBe('none')

    const drop = createDragEvent('drop', dataTransfer)
    expect(document.dispatchEvent(drop)).toBe(false)
    expect(drop.defaultPrevented).toBe(true)
  })

  it('composes with useUnixFSBrowserDrag native upload handlers', () => {
    const upload = vi.fn()
    const { getByTestId } = render(
      <>
        <FileDropGuard />
        <div
          data-testid="unixfs-drop-target"
          onDragOver={(event) => {
            if (!hasNativeFileDrag(event.dataTransfer)) return
            event.preventDefault()
            event.stopPropagation()
            event.dataTransfer.dropEffect = 'copy'
          }}
          onDrop={(event) => {
            if (!hasNativeFileDrag(event.dataTransfer)) return
            event.preventDefault()
            event.stopPropagation()
            upload()
          }}
        />
      </>,
    )
    const dataTransfer = {
      items: [{ kind: 'file' }],
      types: ['Files'],
      dropEffect: 'copy',
    } as unknown as DataTransfer
    const target = getByTestId('unixfs-drop-target')

    const dragOver = createDragEvent('dragover', dataTransfer)
    expect(fireEvent(target, dragOver)).toBe(false)
    expect(dataTransfer.dropEffect).toBe('copy')

    const drop = createDragEvent('drop', dataTransfer)
    expect(fireEvent(target, drop)).toBe(false)
    expect(upload).toHaveBeenCalledOnce()
    expect(dataTransfer.dropEffect).toBe('copy')
  })

  it('does not claim internal app drags and removes its listeners on unmount', () => {
    const { unmount } = render(<FileDropGuard />)
    const appDrag = createDragEvent('dragover', {
      items: [{ kind: 'string' }],
      types: [APP_DRAG_MIME],
      dropEffect: 'move',
    } as unknown as DataTransfer)

    expect(document.dispatchEvent(appDrag)).toBe(true)
    expect(appDrag.defaultPrevented).toBe(false)
    expect(appDrag.dataTransfer?.dropEffect).toBe('move')

    unmount()
    const fileDrop = createDragEvent('drop', {
      items: [{ kind: 'file' }],
      types: ['Files'],
    } as unknown as DataTransfer)
    expect(document.dispatchEvent(fileDrop)).toBe(true)
    expect(fileDrop.defaultPrevented).toBe(false)
  })
})
