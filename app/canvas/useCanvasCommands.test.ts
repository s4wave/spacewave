import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

import { useCanvasCommands } from './useCanvasCommands.js'

const registeredCommands: Array<Record<string, unknown>> = []
const mockOpenCommand = vi.fn()

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: Record<string, unknown>) => {
    registeredCommands.push(opts)
  },
}))

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/contexts/TabActiveContext.js', () => ({
  useIsTabActive: () => true,
}))

describe('useCanvasCommands', () => {
  afterEach(() => {
    registeredCommands.length = 0
    mockOpenCommand.mockReset()
  })

  it('registers the background insertion and view commands', () => {
    const onAddObject = vi.fn()
    const subItems = vi.fn().mockResolvedValue([])

    renderHook(() =>
      useCanvasCommands({
        actions: {
          delete: vi.fn(),
          copy: vi.fn(),
          paste: vi.fn(),
          undo: vi.fn(),
          redo: vi.fn(),
          'select-all': vi.fn(),
          deselect: vi.fn(),
          'zoom-in': vi.fn(),
          'zoom-out': vi.fn(),
          'zoom-reset': vi.fn(),
          'fit-view': vi.fn(),
          'bring-to-front': vi.fn(),
          'send-to-back': vi.fn(),
        },
        moveSelected: vi.fn(),
        selectionFocus: 'border',
        hasSelection: true,
        onToolChange: vi.fn(),
        onCancelDrag: vi.fn(),
        onSetFocus: vi.fn(),
        onAddText: vi.fn(),
        onAddObject,
        addObjectSubItems: subItems,
      }),
    )

    const commandIds = registeredCommands.map((cmd) => cmd.commandId)
    expect(commandIds).toContain('canvas.zoom-reset')
    expect(commandIds).toContain('canvas.add-text')
    expect(commandIds).toContain('canvas.add-object')

    const addObjectCommand = registeredCommands.find(
      (cmd) => cmd.commandId === 'canvas.add-object',
    )
    expect(addObjectCommand?.hasSubItems).toBe(true)
    expect(addObjectCommand?.subItems).toBe(subItems)

    const handler = addObjectCommand?.handler as
      | ((args: Record<string, string>) => void)
      | undefined
    if (!handler) {
      throw new Error('expected add object handler')
    }

    handler({})
    expect(mockOpenCommand).toHaveBeenCalledWith('canvas.add-object')

    handler({ subItemId: 'docs/guide' })
    expect(onAddObject).toHaveBeenCalledWith('docs/guide')
  })
})
