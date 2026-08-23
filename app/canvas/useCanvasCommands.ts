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

// CanvasCommandContext carries the per-mount state the command table's
// predicates and handlers read.
interface CanvasCommandContext {
  isTabActive: boolean
  borderActive: boolean
  contentFocused: boolean
  actions: UseCanvasActionsResult['actions']
  moveSelected: UseCanvasActionsResult['moveSelected']
  openCommand: (commandId: string) => void
  onToolChange?: (tool: CanvasTool) => void
  onCancelDrag?: () => void
  onSetFocus: (focus: SelectionFocus) => void
  onAddText?: () => void
  onAddObject?: (objectKey: string) => void
  addObjectSubItems?: SubItemsCallback
  onAddImage?: (path: string) => void
  addImageSubItems?: SubItemsCallback
}

// CanvasCommandSpec is one entry of the static canvas shortcut table:
// registration fields, an active predicate over the mount context, and the
// behavior to run when invoked.
interface CanvasCommandSpec {
  commandId: string
  label: string
  description?: string
  menuPath?: string
  menuGroup?: number
  menuOrder?: number
  combo?: string
  active: (ctx: CanvasCommandContext) => boolean
  enabled?: (ctx: CanvasCommandContext) => boolean
  hasSubItems?: boolean
  subItems?: (ctx: CanvasCommandContext) => SubItemsCallback | undefined
  run: (ctx: CanvasCommandContext, args: Record<string, string>) => void
}

const tabActive = (ctx: CanvasCommandContext): boolean => ctx.isTabActive
const borderActive = (ctx: CanvasCommandContext): boolean => ctx.borderActive

// CANVAS_COMMANDS is the shortcut table; useCanvasCommands registers each
// entry. The table is module-level so the hook call count stays constant.
const CANVAS_COMMANDS: CanvasCommandSpec[] = [
  // Escape: content-focused switches to border, otherwise deselect.
  {
    commandId: 'canvas.escape',
    label: 'Deselect / Exit Content',
    combo: 'Escape',
    active: tabActive,
    run: (ctx) => {
      if (ctx.contentFocused) {
        ctx.onSetFocus('border')
      } else {
        ctx.onCancelDrag?.()
        ctx.actions.deselect()
      }
    },
  },

  // Edit actions.
  {
    commandId: 'canvas.copy',
    label: 'Copy',
    menuPath: 'Edit/Copy',
    combo: 'CmdOrCtrl+C',
    menuGroup: 20,
    menuOrder: 2,
    active: tabActive,
    run: (ctx) => ctx.actions.copy(),
  },
  {
    commandId: 'canvas.paste',
    label: 'Paste',
    menuPath: 'Edit/Paste',
    combo: 'CmdOrCtrl+V',
    menuGroup: 20,
    menuOrder: 3,
    active: tabActive,
    run: (ctx) => ctx.actions.paste(),
  },
  {
    commandId: 'canvas.undo',
    label: 'Undo',
    menuPath: 'Edit/Undo',
    combo: 'CmdOrCtrl+Z',
    menuGroup: 10,
    menuOrder: 1,
    active: tabActive,
    run: (ctx) => ctx.actions.undo(),
  },
  {
    commandId: 'canvas.redo',
    label: 'Redo',
    menuPath: 'Edit/Redo',
    combo: 'CmdOrCtrl+Shift+Z',
    menuGroup: 10,
    menuOrder: 2,
    active: tabActive,
    run: (ctx) => ctx.actions.redo(),
  },
  {
    commandId: 'canvas.select-all',
    label: 'Select All',
    menuPath: 'Edit/Select All',
    combo: 'CmdOrCtrl+A',
    menuGroup: 30,
    menuOrder: 1,
    active: tabActive,
    run: (ctx) => ctx.actions['select-all'](),
  },
  {
    commandId: 'canvas.deselect',
    label: 'Deselect',
    menuPath: 'Edit/Deselect',
    menuGroup: 30,
    menuOrder: 2,
    active: borderActive,
    run: (ctx) => ctx.actions.deselect(),
  },
  {
    commandId: 'canvas.delete',
    label: 'Delete Selected',
    menuPath: 'Edit/Delete',
    combo: 'Delete',
    menuGroup: 40,
    menuOrder: 1,
    active: borderActive,
    run: (ctx) => ctx.actions.delete(),
  },
  {
    commandId: 'canvas.delete-backspace',
    label: 'Delete Selected',
    combo: 'Backspace',
    active: borderActive,
    run: (ctx) => ctx.actions.delete(),
  },
  {
    commandId: 'canvas.bring-to-front',
    label: 'Bring to Front',
    menuPath: 'Edit/Bring to Front',
    menuGroup: 50,
    menuOrder: 1,
    active: borderActive,
    run: (ctx) => ctx.actions['bring-to-front'](),
  },
  {
    commandId: 'canvas.send-to-back',
    label: 'Send to Back',
    menuPath: 'Edit/Send to Back',
    menuGroup: 50,
    menuOrder: 2,
    active: borderActive,
    run: (ctx) => ctx.actions['send-to-back'](),
  },

  // View actions.
  {
    commandId: 'canvas.zoom-in',
    label: 'Zoom In',
    menuPath: 'View/Zoom In',
    combo: '=',
    menuGroup: 10,
    menuOrder: 1,
    active: borderActive,
    run: (ctx) => ctx.actions['zoom-in'](),
  },
  {
    commandId: 'canvas.zoom-in-plus',
    label: 'Zoom In',
    combo: '+',
    active: borderActive,
    run: (ctx) => ctx.actions['zoom-in'](),
  },
  {
    commandId: 'canvas.zoom-out',
    label: 'Zoom Out',
    menuPath: 'View/Zoom Out',
    combo: '-',
    menuGroup: 10,
    menuOrder: 2,
    active: borderActive,
    run: (ctx) => ctx.actions['zoom-out'](),
  },
  {
    commandId: 'canvas.fit-view',
    label: 'Fit View',
    menuPath: 'View/Fit View',
    combo: 'CmdOrCtrl+0',
    menuGroup: 10,
    menuOrder: 3,
    active: tabActive,
    run: (ctx) => ctx.actions['fit-view'](),
  },
  {
    commandId: 'canvas.organize-nodes',
    label: 'Organize Nodes',
    menuPath: 'View/Organize Nodes',
    menuGroup: 10,
    menuOrder: 4,
    active: tabActive,
    run: (ctx) => ctx.actions['organize-nodes'](),
  },
  {
    commandId: 'canvas.zoom-reset',
    label: 'Zoom to 100%',
    menuPath: 'View/Zoom to 100%',
    menuGroup: 10,
    menuOrder: 5,
    active: tabActive,
    run: (ctx) => ctx.actions['zoom-reset'](),
  },

  // Arrow key movement.
  {
    commandId: 'canvas.move-up',
    label: 'Move Up',
    combo: 'ArrowUp',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(0, -ARROW_STEP),
  },
  {
    commandId: 'canvas.move-down',
    label: 'Move Down',
    combo: 'ArrowDown',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(0, ARROW_STEP),
  },
  {
    commandId: 'canvas.move-left',
    label: 'Move Left',
    combo: 'ArrowLeft',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(-ARROW_STEP, 0),
  },
  {
    commandId: 'canvas.move-right',
    label: 'Move Right',
    combo: 'ArrowRight',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(ARROW_STEP, 0),
  },
  {
    commandId: 'canvas.move-up-fast',
    label: 'Move Up Fast',
    combo: 'Shift+ArrowUp',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(0, -ARROW_STEP * 5),
  },
  {
    commandId: 'canvas.move-down-fast',
    label: 'Move Down Fast',
    combo: 'Shift+ArrowDown',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(0, ARROW_STEP * 5),
  },
  {
    commandId: 'canvas.move-left-fast',
    label: 'Move Left Fast',
    combo: 'Shift+ArrowLeft',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(-ARROW_STEP * 5, 0),
  },
  {
    commandId: 'canvas.move-right-fast',
    label: 'Move Right Fast',
    combo: 'Shift+ArrowRight',
    active: borderActive,
    run: (ctx) => ctx.moveSelected(ARROW_STEP * 5, 0),
  },

  // Tool switches (only when onToolChange is provided).
  ...(
    [
      ['select', 'Select', 'v', 1],
      ['draw', 'Draw', 'd', 2],
      ['line', 'Line', 'l', 3],
      ['arrow', 'Arrow', 'a', 4],
      ['rectangle', 'Rectangle', 'r', 5],
      ['ellipse', 'Ellipse', 'e', 6],
      ['text', 'Text', 't', 3],
      ['object', 'Object', 'o', 4],
    ] as Array<[CanvasTool, string, string, number]>
  ).map(([tool, label, combo, menuOrder]): CanvasCommandSpec => ({
    commandId: `canvas.tool.${tool}`,
    label: `${label} Tool`,
    menuPath: `Tools/${label}`,
    combo,
    menuGroup: 1,
    menuOrder,
    active: (ctx) => ctx.borderActive && !!ctx.onToolChange,
    run: (ctx) => ctx.onToolChange?.(tool),
  })),

  {
    commandId: 'canvas.add-text',
    label: 'Add Text Node',
    menuPath: 'Tools/Add Text Node',
    menuGroup: 2,
    menuOrder: 1,
    active: (ctx) => ctx.isTabActive && !!ctx.onAddText,
    enabled: (ctx) => !!ctx.onAddText,
    run: (ctx) => ctx.onAddText?.(),
  },
  {
    commandId: 'canvas.add-object',
    label: 'Add Existing Object',
    menuPath: 'Tools/Add Existing Object',
    menuGroup: 2,
    menuOrder: 2,
    hasSubItems: true,
    subItems: (ctx) => ctx.addObjectSubItems,
    active: (ctx) =>
      ctx.isTabActive && !!ctx.onAddObject && !!ctx.addObjectSubItems,
    enabled: (ctx) => !!ctx.onAddObject && !!ctx.addObjectSubItems,
    run: (ctx, args) => {
      const objectKey = args.subItemId
      if (objectKey) {
        ctx.onAddObject?.(objectKey)
        return
      }
      ctx.openCommand('canvas.add-object')
    },
  },
  {
    commandId: 'canvas.add-image',
    label: 'Add Image',
    description: 'Place an image from this Space on the Canvas',
    menuPath: 'Tools/Add Image',
    menuGroup: 2,
    menuOrder: 3,
    hasSubItems: true,
    subItems: (ctx) => ctx.addImageSubItems,
    active: (ctx) =>
      ctx.isTabActive && !!ctx.onAddImage && !!ctx.addImageSubItems,
    enabled: (ctx) => !!ctx.onAddImage && !!ctx.addImageSubItems,
    run: (ctx, args) => {
      const path = args.subItemId
      if (path) {
        ctx.onAddImage?.(path)
        return
      }
      ctx.openCommand('canvas.add-image')
    },
  },
]

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

  const ctx: CanvasCommandContext = {
    isTabActive,
    borderActive,
    contentFocused,
    actions,
    moveSelected,
    openCommand,
    onToolChange,
    onCancelDrag,
    onSetFocus,
    onAddText,
    onAddObject,
    addObjectSubItems,
    onAddImage,
    addImageSubItems,
  }

  for (const spec of CANVAS_COMMANDS) {
    // CANVAS_COMMANDS is a module-level constant, so this loop registers the
    // same commands in the same order on every render.
    // eslint-disable-next-line react-hooks/rules-of-hooks
    useCommand({
      commandId: spec.commandId,
      label: spec.label,
      description: spec.description,
      menuPath: spec.menuPath,
      menuGroup: spec.menuGroup,
      menuOrder: spec.menuOrder,
      defaultBindings: spec.combo
        ? [
            {
              id: 'default',
              binding: { case: 'combo', value: { combo: spec.combo } },
              surface: CommandSurface.WEB,
            },
          ]
        : undefined,
      active: spec.active(ctx),
      enabled: spec.enabled?.(ctx),
      hasSubItems: spec.hasSubItems,
      subItems: spec.subItems?.(ctx),
      handler: (args: Record<string, string>) => spec.run(ctx, args),
    })
  }
}
