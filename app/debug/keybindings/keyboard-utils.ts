import {
  normalizeKeyCombo,
  type KeybindingPlatform,
} from '@s4wave/web/command/KeybindingResolver.js'
import { detectPlatform } from '@s4wave/web/platform/detect-platform.js'

const MODIFIER_KEYS: Record<string, true> = {
  Alt: true,
  Control: true,
  Meta: true,
  Shift: true,
}

const KEY_ALIASES: Record<string, string> = {
  ' ': 'Space',
  '+': '=',
  _: '-',
  '{': '[',
  '}': ']',
  ':': ';',
  '"': "'",
  '<': ',',
  '>': '.',
  '?': '/',
  '~': '`',
  '|': '\\',
  ArrowDown: 'ArrowDown',
  ArrowLeft: 'ArrowLeft',
  ArrowRight: 'ArrowRight',
  ArrowUp: 'ArrowUp',
  Esc: 'Escape',
}

const MAC_DISPLAY_TOKENS: Record<string, string> = {
  CmdOrCtrl: '⌘',
  Cmd: '⌘',
  Ctrl: '⌃',
  Alt: '⌥',
  Shift: '⇧',
  ArrowDown: '↓',
  ArrowLeft: '←',
  ArrowRight: '→',
  ArrowUp: '↑',
  Backspace: '⌫',
  Delete: '⌦',
  Enter: '↵',
  Escape: 'Esc',
  Space: 'Space',
  Tab: 'Tab',
}

const OTHER_DISPLAY_TOKENS: Record<string, string> = {
  CmdOrCtrl: 'Ctrl',
  Cmd: 'Meta',
  Ctrl: 'Ctrl',
  Alt: 'Alt',
  Shift: 'Shift',
  ArrowDown: '↓',
  ArrowLeft: '←',
  ArrowRight: '→',
  ArrowUp: '↑',
  Backspace: 'Backspace',
  Delete: 'Delete',
  Enter: 'Enter',
  Escape: 'Esc',
  Space: 'Space',
  Tab: 'Tab',
}

export function currentKeybindingPlatform(
  nav: Navigator | undefined = typeof navigator === 'undefined'
    ? undefined
    : navigator,
): KeybindingPlatform {
  return nav && detectPlatform(nav)?.os === 'macos' ? 'mac' : 'other'
}

export function chordFromKeyboardEvent(
  event: KeyboardEvent,
  platform = currentKeybindingPlatform(),
): string | null {
  if (MODIFIER_KEYS[event.key]) return null

  const key = KEY_ALIASES[event.key] ?? event.key
  const normalizedKey = key.length === 1 ? key.toLocaleUpperCase() : key
  const tokens: string[] = []

  if (event.metaKey) tokens.push(platform === 'mac' ? 'CmdOrCtrl' : 'Cmd')
  if (event.ctrlKey) tokens.push(platform === 'mac' ? 'Ctrl' : 'CmdOrCtrl')
  if (event.altKey) tokens.push('Alt')
  if (event.shiftKey) tokens.push('Shift')
  tokens.push(normalizedKey)

  return tokens.join('+')
}

export function chordDisplayTokens(
  chord: string,
  platform = currentKeybindingPlatform(),
): readonly string[] {
  if (!chord) return []
  const displayTokens =
    platform === 'mac' ? MAC_DISPLAY_TOKENS : OTHER_DISPLAY_TOKENS
  return chord.split('+').map((token) => displayTokens[token] ?? token)
}

export function chordComparisonKey(
  chord: string,
  platform = currentKeybindingPlatform(),
): string {
  return normalizeKeyCombo(chord, platform)
}

export function chordsMatch(
  left: string,
  right: string,
  platform = currentKeybindingPlatform(),
): boolean {
  return (
    left.length > 0 &&
    right.length > 0 &&
    chordComparisonKey(left, platform) === chordComparisonKey(right, platform)
  )
}

export function modifierDisplayName(
  token: string,
  platform = currentKeybindingPlatform(),
): string {
  if (token !== 'CmdOrCtrl') return token
  return platform === 'mac' ? 'command' : 'control'
}
