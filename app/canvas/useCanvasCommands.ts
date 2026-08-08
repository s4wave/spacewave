import { useCallback } from 'react'

import {
  type SubItemsCallback,
  useOpenCommand,
} from '@s4wave/web/command/CommandContext.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'

import type { UseCanvasActionsResult } from './useCanvasActions.js'
import type { CanvasTool } from './types.js'
import type { SelectionFocus } from './useCanvasSelection.js'

// UseCanvasCommandsParams are the parameters for useCanvasCommands.
interface UseCanvasCommandsParams {
  actions: UseCanvasActionsResult['actions']
  moveSelected: UseCanvasActionsResult['moveSelected']
  selectionFocus: SelectionFocus
  hasSelection: boolean
  onToolChange?: (tool: CanvasTool) => void
  onCancelDrag?: () => void
  onSetFocus: (focus: SelectionFocus) => void
  onAddText?: () => void
  onAddObject?: (objectKey: string) => void
  addObjectSubItems?: SubItemsCallback
  onAddImage?: (path: string) => void
  addImageSubItems?: SubItemsCallback
}

// ARROW_STEP is the number of canvas units to move per arrow key press.
const ARROW_STEP = 10

// useCanvasCommands registers all canvas keyboard shortcuts as commands
// via the command system. Commands are scoped to the active canvas tab
// using useIsTabActive().
export function useCanvasCommands(params: UseCanvasCommandsParams): void {
  const {
    actions,
    moveSelected,
    selectionFocus,
    hasSelection,
    onToolChange,
    onCancelDrag,
    onSetFocus,
    onAddText,
    onAddObject,
    addObjectSubItems,
    onAddImage,
    addImageSubItems,
  } = params

  const isTabActive = useIsTabActive()
  const openCommand = useOpenCommand()
  const contentFocused = selectionFocus === 'content' && hasSelection
  const borderActive = isTabActive && !contentFocused

  // Escape: content-focused switches to border, otherwise deselect.
  useCommand({
    commandId: 'canvas.escape',
    label: 'Deselect / Exit Content',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'Escape' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: isTabActive,
    handler: useCallback(() => {
      if (contentFocused) {
        onSetFocus('border')
      } else {
        onCancelDrag?.()
        actions.deselect()
      }
    }, [contentFocused, onSetFocus, onCancelDrag, actions]),
  })

  // Edit actions.
  useCommand({
    commandId: 'canvas.copy',
    label: 'Copy',
    menuPath: 'Edit/Copy',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+C' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 20,
    menuOrder: 2,
    active: isTabActive,
    handler: useCallback(() => {
      actions.copy()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.paste',
    label: 'Paste',
    menuPath: 'Edit/Paste',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+V' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 20,
    menuOrder: 3,
    active: isTabActive,
    handler: useCallback(() => {
      actions.paste()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.undo',
    label: 'Undo',
    menuPath: 'Edit/Undo',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+Z' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 10,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      actions.undo()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.redo',
    label: 'Redo',
    menuPath: 'Edit/Redo',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+Shift+Z' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 10,
    menuOrder: 2,
    active: isTabActive,
    handler: useCallback(() => {
      actions.redo()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.select-all',
    label: 'Select All',
    menuPath: 'Edit/Select All',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+A' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 30,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      actions['select-all']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.deselect',
    label: 'Deselect',
    menuPath: 'Edit/Deselect',
    menuGroup: 30,
    menuOrder: 2,
    active: borderActive,
    handler: useCallback(() => {
      actions.deselect()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.delete',
    label: 'Delete Selected',
    menuPath: 'Edit/Delete',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'Delete' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 40,
    menuOrder: 1,
    active: borderActive,
    handler: useCallback(() => {
      actions.delete()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.delete-backspace',
    label: 'Delete Selected',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'Backspace' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      actions.delete()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.bring-to-front',
    label: 'Bring to Front',
    menuPath: 'Edit/Bring to Front',
    menuGroup: 50,
    menuOrder: 1,
    active: borderActive,
    handler: useCallback(() => {
      actions['bring-to-front']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.send-to-back',
    label: 'Send to Back',
    menuPath: 'Edit/Send to Back',
    menuGroup: 50,
    menuOrder: 2,
    active: borderActive,
    handler: useCallback(() => {
      actions['send-to-back']()
    }, [actions]),
  })

  // View actions.
  useCommand({
    commandId: 'canvas.zoom-in',
    label: 'Zoom In',
    menuPath: 'View/Zoom In',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: '=' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 10,
    menuOrder: 1,
    active: borderActive,
    handler: useCallback(() => {
      actions['zoom-in']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.zoom-in-plus',
    label: 'Zoom In',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: '+' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      actions['zoom-in']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.zoom-out',
    label: 'Zoom Out',
    menuPath: 'View/Zoom Out',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: '-' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 10,
    menuOrder: 2,
    active: borderActive,
    handler: useCallback(() => {
      actions['zoom-out']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.fit-view',
    label: 'Fit View',
    menuPath: 'View/Fit View',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+0' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 10,
    menuOrder: 3,
    active: isTabActive,
    handler: useCallback(() => {
      actions['fit-view']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.zoom-reset',
    label: 'Zoom to 100%',
    menuPath: 'View/Zoom to 100%',
    menuGroup: 10,
    menuOrder: 5,
    active: isTabActive,
    handler: useCallback(() => {
      actions['zoom-reset']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.organize-nodes',
    label: 'Organize Nodes',
    menuPath: 'View/Organize Nodes',
    menuGroup: 10,
    menuOrder: 4,
    active: isTabActive,
    handler: useCallback(() => {
      actions['organize-nodes']()
    }, [actions]),
  })

  // Arrow key movement.
  useCommand({
    commandId: 'canvas.move-up',
    label: 'Move Up',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'ArrowUp' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, -ARROW_STEP)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-down',
    label: 'Move Down',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'ArrowDown' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, ARROW_STEP)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-left',
    label: 'Move Left',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'ArrowLeft' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(-ARROW_STEP, 0)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-right',
    label: 'Move Right',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'ArrowRight' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(ARROW_STEP, 0)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-up-fast',
    label: 'Move Up Fast',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'Shift+ArrowUp' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, -ARROW_STEP * 5)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-down-fast',
    label: 'Move Down Fast',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'Shift+ArrowDown' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, ARROW_STEP * 5)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-left-fast',
    label: 'Move Left Fast',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'Shift+ArrowLeft' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(-ARROW_STEP * 5, 0)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-right-fast',
    label: 'Move Right Fast',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'Shift+ArrowRight' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(ARROW_STEP * 5, 0)
    }, [moveSelected]),
  })

  // Tool switches (only when onToolChange is provided).
  useCommand({
    commandId: 'canvas.tool.select',
    label: 'Select Tool',
    menuPath: 'Tools/Select',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'v' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 1,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('select')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.draw',
    label: 'Draw Tool',
    menuPath: 'Tools/Draw',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'd' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 2,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('draw')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.line',
    label: 'Line Tool',
    menuPath: 'Tools/Line',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'l' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 3,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('line')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.arrow',
    label: 'Arrow Tool',
    menuPath: 'Tools/Arrow',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'a' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 4,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('arrow')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.rectangle',
    label: 'Rectangle Tool',
    menuPath: 'Tools/Rectangle',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'r' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 5,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('rectangle')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.ellipse',
    label: 'Ellipse Tool',
    menuPath: 'Tools/Ellipse',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'e' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 6,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('ellipse')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.text',
    label: 'Text Tool',
    menuPath: 'Tools/Text',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 't' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 3,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('text')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.object',
    label: 'Object Tool',
    menuPath: 'Tools/Object',
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'o' } },
        surface: CommandSurface.WEB,
      },
    ],
    menuGroup: 1,
    menuOrder: 4,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('object')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.add-text',
    label: 'Add Text Node',
    menuPath: 'Tools/Add Text Node',
    menuGroup: 2,
    menuOrder: 1,
    active: isTabActive && !!onAddText,
    enabled: !!onAddText,
    handler: useCallback(() => {
      onAddText?.()
    }, [onAddText]),
  })

  useCommand({
    commandId: 'canvas.add-object',
    label: 'Add Existing Object',
    menuPath: 'Tools/Add Existing Object',
    menuGroup: 2,
    menuOrder: 2,
    active: isTabActive && !!onAddObject && !!addObjectSubItems,
    enabled: !!onAddObject && !!addObjectSubItems,
    hasSubItems: true,
    subItems: addObjectSubItems,
    handler: useCallback(
      (args: Record<string, string>) => {
        const objectKey = args.subItemId
        if (objectKey) {
          onAddObject?.(objectKey)
          return
        }
        openCommand('canvas.add-object')
      },
      [onAddObject, openCommand],
    ),
  })

  useCommand({
    commandId: 'canvas.add-image',
    label: 'Add Image',
    description: 'Place an image from this Space on the Canvas',
    menuPath: 'Tools/Add Image',
    menuGroup: 2,
    menuOrder: 3,
    active: isTabActive && !!onAddImage && !!addImageSubItems,
    enabled: !!onAddImage && !!addImageSubItems,
    hasSubItems: true,
    subItems: addImageSubItems,
    handler: useCallback(
      (args: Record<string, string>) => {
        const path = args.subItemId
        if (path) {
          onAddImage?.(path)
          return
        }
        openCommand('canvas.add-image')
      },
      [onAddImage, openCommand],
    ),
  })
}
