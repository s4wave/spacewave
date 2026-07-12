import type { KeybindingCommand } from './keybindings-model.js'
import { currentKeybindingPlatform } from './keyboard-utils.js'

export interface KeyboardKeyDefinition {
  key: string
  label?: string
  grow?: number
  side?: 'left' | 'right'
}

export interface KeyboardRowDefinition {
  id: string
  keys: readonly KeyboardKeyDefinition[]
}

const COMMON_KEYBOARD_ROWS: readonly KeyboardRowDefinition[] = [
  {
    id: 'function',
    keys: [
      { key: 'Escape', label: 'esc', grow: 1.4 },
      { key: 'F1' },
      { key: 'F2' },
      { key: 'F3' },
      { key: 'F4' },
      { key: 'F5' },
      { key: 'F6' },
      { key: 'F7' },
      { key: 'F8' },
      { key: 'F9' },
      { key: 'F10' },
      { key: 'F11' },
      { key: 'F12' },
    ],
  },
  {
    id: 'number',
    keys: [
      { key: '`' },
      { key: '1' },
      { key: '2' },
      { key: '3' },
      { key: '4' },
      { key: '5' },
      { key: '6' },
      { key: '7' },
      { key: '8' },
      { key: '9' },
      { key: '0' },
      { key: '-' },
      { key: '=' },
      { key: 'Backspace', label: 'delete', grow: 2 },
    ],
  },
  {
    id: 'top',
    keys: [
      { key: 'Tab', label: 'tab', grow: 1.5 },
      { key: 'Q' },
      { key: 'W' },
      { key: 'E' },
      { key: 'R' },
      { key: 'T' },
      { key: 'Y' },
      { key: 'U' },
      { key: 'I' },
      { key: 'O' },
      { key: 'P' },
      { key: '[' },
      { key: ']' },
      { key: '\\', grow: 1.5 },
    ],
  },
  {
    id: 'home',
    keys: [
      { key: 'CapsLock', label: 'caps', grow: 1.8 },
      { key: 'A' },
      { key: 'S' },
      { key: 'D' },
      { key: 'F' },
      { key: 'G' },
      { key: 'H' },
      { key: 'J' },
      { key: 'K' },
      { key: 'L' },
      { key: ';' },
      { key: "'" },
      { key: 'Enter', label: 'return', grow: 2.2 },
    ],
  },
  {
    id: 'bottom',
    keys: [
      { key: 'Shift', label: 'shift', grow: 2.3, side: 'left' },
      { key: 'Z' },
      { key: 'X' },
      { key: 'C' },
      { key: 'V' },
      { key: 'B' },
      { key: 'N' },
      { key: 'M' },
      { key: ',' },
      { key: '.' },
      { key: '/' },
      { key: 'ArrowUp', label: '↑' },
      { key: 'Shift', label: 'shift', grow: 1.5, side: 'right' },
    ],
  },
]

const MAC_KEYBOARD_ROWS: readonly KeyboardRowDefinition[] = [
  ...COMMON_KEYBOARD_ROWS,
  {
    id: 'modifier',
    keys: [
      { key: 'Ctrl', label: 'control', grow: 1.5 },
      { key: 'Alt', label: 'option', grow: 1.5 },
      { key: 'CmdOrCtrl', grow: 1.8, side: 'left' },
      { key: 'Space', label: '', grow: 6 },
      { key: 'CmdOrCtrl', grow: 1.8, side: 'right' },
      { key: 'Alt', label: 'option', grow: 1.5 },
      { key: 'ArrowLeft', label: '←' },
      { key: 'ArrowDown', label: '↓' },
      { key: 'ArrowRight', label: '→' },
    ],
  },
]

const OTHER_KEYBOARD_ROWS: readonly KeyboardRowDefinition[] = [
  ...COMMON_KEYBOARD_ROWS,
  {
    id: 'modifier',
    keys: [
      { key: 'CmdOrCtrl', label: 'control', grow: 1.8 },
      { key: 'Cmd', label: 'meta', grow: 1.5 },
      { key: 'Alt', label: 'alt', grow: 1.5, side: 'left' },
      { key: 'Space', label: '', grow: 6 },
      { key: 'Alt', label: 'alt', grow: 1.5, side: 'right' },
      { key: 'ArrowLeft', label: '←' },
      { key: 'ArrowDown', label: '↓' },
      { key: 'ArrowRight', label: '→' },
    ],
  },
]

export function keyboardRowsForPlatform(
  platform = currentKeybindingPlatform(),
): readonly KeyboardRowDefinition[] {
  return platform === 'mac' ? MAC_KEYBOARD_ROWS : OTHER_KEYBOARD_ROWS
}

const MODIFIER_TOKENS: Record<string, true> = {
  Alt: true,
  Cmd: true,
  CmdOrCtrl: true,
  Ctrl: true,
  Shift: true,
}

export function commandUsesKey(
  command: KeybindingCommand,
  key: string,
  platform = currentKeybindingPlatform(),
): boolean {
  const tokens = command.binding.split('+')
  if (platform !== 'mac' && key === 'CmdOrCtrl') {
    return tokens.includes('CmdOrCtrl') || tokens.includes('Ctrl')
  }
  if (MODIFIER_TOKENS[key]) return tokens.includes(key)
  return (
    tokens[tokens.length - 1]?.toLocaleLowerCase() === key.toLocaleLowerCase()
  )
}
