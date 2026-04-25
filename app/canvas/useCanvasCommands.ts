import { useCallback } from 'react'

import {
  type SubItemsCallback,
  useOpenCommand,
} from '@s4wave/web/command/CommandContext.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
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
  } = params

  const isTabActive = useIsTabActive()
  const openCommand = useOpenCommand()
  const contentFocused = selectionFocus === 'content' && hasSelection
  const borderActive = isTabActive && !contentFocused

  // Escape: content-focused switches to border, otherwise deselect.
  useCommand({
    commandId: 'canvas.escape',
    label: 'Deselect / Exit Content',
    keybinding: 'Escape',
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
    keybinding: 'CmdOrCtrl+C',
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
    keybinding: 'CmdOrCtrl+V',
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
    keybinding: 'CmdOrCtrl+Z',
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
    keybinding: 'CmdOrCtrl+Shift+Z',
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
    keybinding: 'CmdOrCtrl+A',
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
    keybinding: 'Delete',
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
    keybinding: 'Backspace',
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
    keybinding: '=',
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
    keybinding: '+',
    active: borderActive,
    handler: useCallback(() => {
      actions['zoom-in']()
    }, [actions]),
  })

  useCommand({
    commandId: 'canvas.zoom-out',
    label: 'Zoom Out',
    menuPath: 'View/Zoom Out',
    keybinding: '-',
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
    keybinding: 'CmdOrCtrl+0',
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
    menuOrder: 4,
    active: isTabActive,
    handler: useCallback(() => {
      actions['zoom-reset']()
    }, [actions]),
  })

  // Arrow key movement.
  useCommand({
    commandId: 'canvas.move-up',
    label: 'Move Up',
    keybinding: 'ArrowUp',
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, -ARROW_STEP)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-down',
    label: 'Move Down',
    keybinding: 'ArrowDown',
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, ARROW_STEP)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-left',
    label: 'Move Left',
    keybinding: 'ArrowLeft',
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(-ARROW_STEP, 0)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-right',
    label: 'Move Right',
    keybinding: 'ArrowRight',
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(ARROW_STEP, 0)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-up-fast',
    label: 'Move Up Fast',
    keybinding: 'Shift+ArrowUp',
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, -ARROW_STEP * 5)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-down-fast',
    label: 'Move Down Fast',
    keybinding: 'Shift+ArrowDown',
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(0, ARROW_STEP * 5)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-left-fast',
    label: 'Move Left Fast',
    keybinding: 'Shift+ArrowLeft',
    active: borderActive,
    handler: useCallback(() => {
      moveSelected(-ARROW_STEP * 5, 0)
    }, [moveSelected]),
  })

  useCommand({
    commandId: 'canvas.move-right-fast',
    label: 'Move Right Fast',
    keybinding: 'Shift+ArrowRight',
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
    keybinding: 'v',
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
    keybinding: 'd',
    menuGroup: 1,
    menuOrder: 2,
    active: borderActive && !!onToolChange,
    handler: useCallback(() => {
      onToolChange?.('draw')
    }, [onToolChange]),
  })

  useCommand({
    commandId: 'canvas.tool.text',
    label: 'Text Tool',
    menuPath: 'Tools/Text',
    keybinding: 't',
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
    keybinding: 'o',
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
}
