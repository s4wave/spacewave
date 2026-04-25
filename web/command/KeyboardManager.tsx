import { useCallback, useEffect, useMemo } from 'react'

import { useCommands, useInvokeCommand } from './CommandContext.js'

// isMacPlatform detects whether the current platform is macOS.
const isMacPlatform =
  typeof navigator !== 'undefined' && navigator.platform.includes('Mac')

// ParsedKeybinding is a parsed keyboard shortcut combination.
interface ParsedKeybinding {
  key: string
  meta: boolean
  ctrl: boolean
  shift: boolean
  alt: boolean
}

// parseKeybinding converts a keybinding string like "CmdOrCtrl+Shift+S"
// into a structured ParsedKeybinding.
function parseKeybinding(binding: string): ParsedKeybinding {
  const parts = binding.split('+')
  let meta = false
  let ctrl = false
  let shift = false
  let alt = false
  let key = ''

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i]
    if (i === parts.length - 1) {
      key = part.toLowerCase()
      continue
    }
    switch (part) {
      case 'CmdOrCtrl':
        if (isMacPlatform) {
          meta = true
        } else {
          ctrl = true
        }
        break
      case 'Cmd':
        meta = true
        break
      case 'Ctrl':
        ctrl = true
        break
      case 'Shift':
        shift = true
        break
      case 'Alt':
        alt = true
        break
    }
  }

  return { key, meta, ctrl, shift, alt }
}

// buildComboString builds a normalized combo string from modifier flags and key.
function buildComboString(
  meta: boolean,
  ctrl: boolean,
  alt: boolean,
  shift: boolean,
  key: string,
): string {
  let combo = ''
  if (meta) combo += 'meta+'
  if (ctrl) combo += 'ctrl+'
  if (alt) combo += 'alt+'
  if (shift) combo += 'shift+'
  combo += key
  return combo
}

// KeyboardManager listens for keyboard events and dispatches matching commands.
// Renders nothing -- uses document-level event listener in capture phase.
export function KeyboardManager() {
  const commands = useCommands()
  const invokeCommand = useInvokeCommand()

  const keybindingMap = useMemo(() => {
    const map = new Map<string, string>()

    for (const cmd of commands) {
      const binding = cmd.command?.keybinding
      const commandId = cmd.command?.commandId
      if (!binding || !commandId || !cmd.active || cmd.enabled === false) {
        continue
      }

      const parsed = parseKeybinding(binding)
      const combo = buildComboString(
        parsed.meta,
        parsed.ctrl,
        parsed.alt,
        parsed.shift,
        parsed.key,
      )
      map.set(combo, commandId)
    }
    return map
  }, [commands])

  const handler = useCallback(
    (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      if (
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable
      ) {
        return
      }

      const combo = buildComboString(
        e.metaKey,
        e.ctrlKey,
        e.altKey,
        e.shiftKey,
        e.key.toLowerCase(),
      )
      const commandId = keybindingMap.get(combo)
      if (!commandId) return

      e.preventDefault()
      invokeCommand(commandId)
    },
    [keybindingMap, invokeCommand],
  )

  useEffect(() => {
    document.addEventListener('keydown', handler, true)
    return () => document.removeEventListener('keydown', handler, true)
  }, [handler])

  return null
}
