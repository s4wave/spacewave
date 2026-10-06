import { useReducer, type Dispatch } from 'react'

import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

import type { ContextMenuState } from './UnixFSContextMenu.js'
import type { UnixFSMoveItem } from './move.js'

export interface UnixFSBrowserState {
  pendingName: string | null
  contextMenu: ContextMenuState | null
  newFolderName: string | null
  newFileName: string | null
  deleteTargets: FileEntry[] | null
  moveDialogItems: UnixFSMoveItem[] | null
  renamingEntry: FileEntry | null
  isDragging: boolean
  folderDropEntryId: string | null
}

export type UnixFSBrowserAction =
  | { type: 'set-pending-name'; name: string | null }
  | { type: 'set-context-menu'; menu: ContextMenuState | null }
  | { type: 'start-rename'; entry: FileEntry }
  | { type: 'clear-rename' }
  | { type: 'request-delete'; entries: FileEntry[] }
  | { type: 'clear-delete' }
  | { type: 'request-move'; items: UnixFSMoveItem[] }
  | { type: 'clear-move' }
  | { type: 'start-new-folder' }
  | { type: 'set-new-folder-name'; name: string }
  | { type: 'clear-new-folder' }
  | { type: 'start-new-file' }
  | { type: 'set-new-file-name'; name: string }
  | { type: 'clear-new-file' }
  | { type: 'set-dragging'; dragging: boolean }
  | { type: 'set-folder-drop-entry'; id: string | null }

export type UnixFSBrowserDispatch = Dispatch<UnixFSBrowserAction>

const initialUnixFSBrowserState: UnixFSBrowserState = {
  pendingName: null,
  contextMenu: null,
  newFolderName: null,
  newFileName: null,
  deleteTargets: null,
  moveDialogItems: null,
  renamingEntry: null,
  isDragging: false,
  folderDropEntryId: null,
}

function unixFSBrowserReducer(
  state: UnixFSBrowserState,
  action: UnixFSBrowserAction,
): UnixFSBrowserState {
  switch (action.type) {
    case 'set-pending-name':
      return { ...state, pendingName: action.name }
    case 'set-context-menu':
      return { ...state, contextMenu: action.menu }
    case 'start-rename':
      return { ...state, renamingEntry: action.entry }
    case 'clear-rename':
      return { ...state, renamingEntry: null }
    case 'request-delete':
      return { ...state, deleteTargets: action.entries }
    case 'clear-delete':
      return { ...state, deleteTargets: null }
    case 'request-move':
      return {
        ...state,
        contextMenu: null,
        moveDialogItems: action.items,
      }
    case 'clear-move':
      return { ...state, moveDialogItems: null }
    case 'start-new-folder':
      return {
        ...state,
        contextMenu: null,
        newFolderName: '',
        newFileName: null,
        renamingEntry: null,
      }
    case 'set-new-folder-name':
      return { ...state, newFolderName: action.name }
    case 'clear-new-folder':
      return { ...state, newFolderName: null }
    case 'start-new-file':
      return {
        ...state,
        contextMenu: null,
        newFolderName: null,
        newFileName: '',
        renamingEntry: null,
      }
    case 'set-new-file-name':
      return { ...state, newFileName: action.name }
    case 'clear-new-file':
      return { ...state, newFileName: null }
    case 'set-dragging':
      return { ...state, isDragging: action.dragging }
    case 'set-folder-drop-entry':
      return { ...state, folderDropEntryId: action.id }
  }
}

/** useUnixFSBrowserState holds the transient UI state of the browser. */
export function useUnixFSBrowserState() {
  return useReducer(unixFSBrowserReducer, initialUnixFSBrowserState)
}
