import {
  chordDisplayTokens,
  currentKeybindingPlatform,
} from './keyboard-utils.js'

export type KeybindingContext =
  | 'Global'
  | 'Shell tab'
  | 'Editor'
  | 'List'
  | 'Canvas'
  | 'Modal'
  | 'Text input'

export type KeybindingCategory =
  | 'General'
  | 'Navigation'
  | 'View'
  | 'Editing'
  | 'Files'
  | 'Terminal'
  | 'Canvas'
  | 'Collaboration'

export interface KeybindingCommandDefinition {
  id: string
  label: string
  description: string
  category: KeybindingCategory
  context: KeybindingContext
  defaultBinding: string
  keywords: readonly string[]
}

export interface KeybindingCommand extends KeybindingCommandDefinition {
  binding: string
}

export const KEYBINDING_CONTEXTS: readonly KeybindingContext[] = [
  'Global',
  'Shell tab',
  'Editor',
  'List',
  'Canvas',
  'Modal',
  'Text input',
]

export const KEYBINDING_CATEGORIES: readonly KeybindingCategory[] = [
  'General',
  'Navigation',
  'View',
  'Editing',
  'Files',
  'Terminal',
  'Canvas',
  'Collaboration',
]

export const MOCK_KEYBINDING_COMMANDS: readonly KeybindingCommandDefinition[] =
  [
    {
      id: 'command.finder',
      label: 'Open command finder',
      description: 'Search every action available in the current workspace.',
      category: 'General',
      context: 'Global',
      defaultBinding: 'CmdOrCtrl+K',
      keywords: ['palette', 'actions', 'search'],
    },
    {
      id: 'command.settings',
      label: 'Open settings',
      description: 'Open Spacewave preferences and account settings.',
      category: 'General',
      context: 'Global',
      defaultBinding: 'CmdOrCtrl+,',
      keywords: ['preferences', 'configure'],
    },
    {
      id: 'command.quickOpen',
      label: 'Quick open',
      description: 'Find a space, object, or recent resource.',
      category: 'Navigation',
      context: 'Global',
      defaultBinding: 'CmdOrCtrl+P',
      keywords: ['go', 'resource', 'recent'],
    },
    {
      id: 'command.notifications',
      label: 'Show notifications',
      description: 'Open the notification and activity inbox.',
      category: 'General',
      context: 'Global',
      defaultBinding: 'CmdOrCtrl+Shift+N',
      keywords: ['inbox', 'activity'],
    },
    {
      id: 'workspace.new',
      label: 'New workspace tab',
      description: 'Create a blank tab in the active window.',
      category: 'General',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+T',
      keywords: ['tab', 'create'],
    },
    {
      id: 'workspace.close',
      label: 'Close workspace tab',
      description: 'Close the active workspace tab.',
      category: 'General',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+W',
      keywords: ['tab', 'dismiss'],
    },
    {
      id: 'workspace.reopen',
      label: 'Reopen closed tab',
      description: 'Restore the most recently closed workspace tab.',
      category: 'General',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+Shift+T',
      keywords: ['restore', 'history'],
    },
    {
      id: 'workspace.next',
      label: 'Next workspace tab',
      description: 'Move focus to the tab on the right.',
      category: 'Navigation',
      context: 'Shell tab',
      defaultBinding: 'Ctrl+Tab',
      keywords: ['switch', 'right'],
    },
    {
      id: 'workspace.previous',
      label: 'Previous workspace tab',
      description: 'Move focus to the tab on the left.',
      category: 'Navigation',
      context: 'Shell tab',
      defaultBinding: 'Ctrl+Shift+Tab',
      keywords: ['switch', 'left'],
    },
    {
      id: 'workspace.focusSidebar',
      label: 'Focus sidebar',
      description: 'Move keyboard focus into the object sidebar.',
      category: 'Navigation',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+0',
      keywords: ['tree', 'objects'],
    },
    {
      id: 'workspace.toggleSidebar',
      label: 'Toggle sidebar',
      description: 'Show or hide the object sidebar.',
      category: 'View',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+B',
      keywords: ['tree', 'panel'],
    },
    {
      id: 'workspace.toggleBottomBar',
      label: 'Toggle bottom panel',
      description: 'Show or hide the activity panel at the bottom.',
      category: 'View',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+J',
      keywords: ['terminal', 'output', 'panel'],
    },
    {
      id: 'terminal.focus',
      label: 'Focus terminal panel',
      description: 'Reveal the bottom panel and focus its active terminal.',
      category: 'Terminal',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+J',
      keywords: ['shell', 'panel', 'focus'],
    },
    {
      id: 'workspace.zen',
      label: 'Toggle focus mode',
      description: 'Hide surrounding chrome and focus on the active object.',
      category: 'View',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+Shift+F',
      keywords: ['zen', 'distraction'],
    },
    {
      id: 'navigation.back',
      label: 'Navigate back',
      description: 'Return to the previous object in navigation history.',
      category: 'Navigation',
      context: 'Global',
      defaultBinding: 'CmdOrCtrl+[',
      keywords: ['history', 'previous'],
    },
    {
      id: 'navigation.forward',
      label: 'Navigate forward',
      description: 'Advance to the next object in navigation history.',
      category: 'Navigation',
      context: 'Global',
      defaultBinding: 'CmdOrCtrl+]',
      keywords: ['history', 'next'],
    },
    {
      id: 'navigation.space',
      label: 'Go to space',
      description: 'Open the space switcher.',
      category: 'Navigation',
      context: 'Global',
      defaultBinding: 'CmdOrCtrl+G',
      keywords: ['switcher', 'home'],
    },
    {
      id: 'editor.save',
      label: 'Save active object',
      description: 'Save pending edits to the active object.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'CmdOrCtrl+S',
      keywords: ['write', 'commit'],
    },
    {
      id: 'editor.saveAll',
      label: 'Save all objects',
      description: 'Save edits in every open object.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'CmdOrCtrl+Alt+S',
      keywords: ['write', 'all'],
    },
    {
      id: 'editor.undo',
      label: 'Undo edit',
      description: 'Undo the most recent editor action.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'CmdOrCtrl+Z',
      keywords: ['history', 'revert'],
    },
    {
      id: 'editor.redo',
      label: 'Redo edit',
      description: 'Reapply the most recently undone editor action.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'CmdOrCtrl+Shift+Z',
      keywords: ['history', 'repeat'],
    },
    {
      id: 'editor.find',
      label: 'Find in object',
      description: 'Search text inside the active editor.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'CmdOrCtrl+F',
      keywords: ['search', 'text'],
    },
    {
      id: 'editor.replace',
      label: 'Find and replace',
      description: 'Search and replace text inside the active editor.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'CmdOrCtrl+Alt+F',
      keywords: ['search', 'change'],
    },
    {
      id: 'editor.comment',
      label: 'Toggle line comment',
      description: 'Comment or uncomment the current line or selection.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'CmdOrCtrl+/',
      keywords: ['code', 'annotation'],
    },
    {
      id: 'editor.format',
      label: 'Format object',
      description: 'Format the active object with its configured formatter.',
      category: 'Editing',
      context: 'Editor',
      defaultBinding: 'Shift+Alt+F',
      keywords: ['code', 'prettify'],
    },
    {
      id: 'editor.definition',
      label: 'Go to definition',
      description: 'Open the definition of the symbol under the cursor.',
      category: 'Navigation',
      context: 'Editor',
      defaultBinding: 'F12',
      keywords: ['symbol', 'source'],
    },
    {
      id: 'terminal.new',
      label: 'New terminal',
      description: 'Create another terminal session in the bottom panel.',
      category: 'Terminal',
      context: 'Shell tab',
      defaultBinding: 'Ctrl+Shift+`',
      keywords: ['shell', 'session'],
    },
    {
      id: 'terminal.clear',
      label: 'Clear terminal',
      description: 'Clear visible output from the active terminal.',
      category: 'Terminal',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+K',
      keywords: ['shell', 'output'],
    },
    {
      id: 'terminal.find',
      label: 'Find in terminal',
      description: 'Search output in the active terminal buffer.',
      category: 'Terminal',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+F',
      keywords: ['shell', 'search'],
    },
    {
      id: 'terminal.next',
      label: 'Next terminal',
      description: 'Focus the next open terminal session.',
      category: 'Terminal',
      context: 'Shell tab',
      defaultBinding: 'Alt+ArrowRight',
      keywords: ['shell', 'switch'],
    },
    {
      id: 'terminal.previous',
      label: 'Previous terminal',
      description: 'Focus the previous open terminal session.',
      category: 'Terminal',
      context: 'Shell tab',
      defaultBinding: 'Alt+ArrowLeft',
      keywords: ['shell', 'switch'],
    },
    {
      id: 'files.newFile',
      label: 'New file',
      description: 'Create a file in the active directory.',
      category: 'Files',
      context: 'List',
      defaultBinding: 'CmdOrCtrl+N',
      keywords: ['create', 'document'],
    },
    {
      id: 'files.newFolder',
      label: 'New folder',
      description: 'Create a directory in the active directory.',
      category: 'Files',
      context: 'List',
      defaultBinding: 'CmdOrCtrl+Shift+N',
      keywords: ['create', 'directory'],
    },
    {
      id: 'files.rename',
      label: 'Rename selected item',
      description: 'Rename the selected file or directory.',
      category: 'Files',
      context: 'List',
      defaultBinding: 'F2',
      keywords: ['change', 'name'],
    },
    {
      id: 'files.delete',
      label: 'Move selected item to trash',
      description: 'Move the selected file or directory to trash.',
      category: 'Files',
      context: 'List',
      defaultBinding: 'CmdOrCtrl+Backspace',
      keywords: ['remove', 'trash'],
    },
    {
      id: 'files.reveal',
      label: 'Reveal in file browser',
      description: 'Reveal the active object in the file browser.',
      category: 'Files',
      context: 'List',
      defaultBinding: 'CmdOrCtrl+Shift+R',
      keywords: ['finder', 'explorer'],
    },
    {
      id: 'canvas.selectAll',
      label: 'Select all nodes',
      description: 'Select every node on the active canvas.',
      category: 'Canvas',
      context: 'Canvas',
      defaultBinding: 'CmdOrCtrl+A',
      keywords: ['graph', 'nodes'],
    },
    {
      id: 'canvas.zoomIn',
      label: 'Zoom in',
      description: 'Increase canvas magnification.',
      category: 'Canvas',
      context: 'Canvas',
      defaultBinding: 'CmdOrCtrl+=',
      keywords: ['graph', 'scale'],
    },
    {
      id: 'canvas.zoomOut',
      label: 'Zoom out',
      description: 'Decrease canvas magnification.',
      category: 'Canvas',
      context: 'Canvas',
      defaultBinding: 'CmdOrCtrl+-',
      keywords: ['graph', 'scale'],
    },
    {
      id: 'canvas.fit',
      label: 'Fit canvas to view',
      description: 'Frame every visible node inside the viewport.',
      category: 'Canvas',
      context: 'Canvas',
      defaultBinding: 'Shift+1',
      keywords: ['graph', 'frame'],
    },
    {
      id: 'canvas.connect',
      label: 'Connect selected nodes',
      description: 'Create a link between the selected nodes.',
      category: 'Canvas',
      context: 'Canvas',
      defaultBinding: 'CmdOrCtrl+L',
      keywords: ['graph', 'edge', 'link'],
    },
    {
      id: 'canvas.duplicate',
      label: 'Duplicate selection',
      description: 'Duplicate selected nodes and their local links.',
      category: 'Canvas',
      context: 'Canvas',
      defaultBinding: 'CmdOrCtrl+D',
      keywords: ['graph', 'copy'],
    },
    {
      id: 'collaboration.share',
      label: 'Share active space',
      description: 'Open sharing controls for the current space.',
      category: 'Collaboration',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+Shift+S',
      keywords: ['invite', 'members'],
    },
    {
      id: 'collaboration.copyLink',
      label: 'Copy object link',
      description: 'Copy a link to the active object.',
      category: 'Collaboration',
      context: 'Shell tab',
      defaultBinding: 'CmdOrCtrl+Shift+C',
      keywords: ['share', 'url'],
    },
    {
      id: 'collaboration.follow',
      label: 'Follow collaborator',
      description: 'Follow a collaborator’s position in the active space.',
      category: 'Collaboration',
      context: 'Shell tab',
      defaultBinding: '',
      keywords: ['presence', 'member'],
    },
    {
      id: 'collaboration.cursor',
      label: 'Toggle collaborator cursors',
      description: 'Show or hide live collaborator cursors.',
      category: 'Collaboration',
      context: 'Shell tab',
      defaultBinding: '',
      keywords: ['presence', 'view'],
    },
  ]

export function commandMatchesQuery(
  command: KeybindingCommand,
  query: string,
): boolean {
  const terms = query.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return true

  const platform = currentKeybindingPlatform()
  const bindingDisplayTokens = chordDisplayTokens(command.binding, platform)
  const defaultDisplayTokens = chordDisplayTokens(
    command.defaultBinding,
    platform,
  )
  const haystack = [
    command.id,
    command.label,
    command.description,
    command.category,
    command.context,
    command.binding,
    command.defaultBinding,
    bindingDisplayTokens.join(''),
    bindingDisplayTokens.join('+'),
    bindingDisplayTokens.join(' '),
    defaultDisplayTokens.join(''),
    defaultDisplayTokens.join('+'),
    defaultDisplayTokens.join(' '),
    ...command.keywords,
  ]
    .join(' ')
    .toLocaleLowerCase()

  return terms.every((term) => haystack.includes(term))
}

export function contextsOverlap(
  left: KeybindingContext,
  right: KeybindingContext,
): boolean {
  return left === right
}
