import type { MenuItemConstructorOptions } from 'electron'
import { describe, expect, it } from 'vitest'

import { buildApplicationMenuTemplate } from './app-menu.js'

describe('buildApplicationMenuTemplate', () => {
  it('disables Electron accelerator registration recursively for renderer-owned shortcuts', () => {
    const template = buildApplicationMenuTemplate({
      appName: 'Spacewave',
      isDebug: true,
      isMac: true,
    })

    const shortcutItems = collectShortcutMenuItems(template)
    const shortcutItemNames = shortcutItems.map(describeMenuItem)
    expect(shortcutItemNames).toEqual(
      expect.arrayContaining([
        '0 label:Spacewave',
        '0.0 role:about',
        '0.8 role:quit',
        '1 label:Edit',
        '1.0 role:undo',
        '1.9 role:selectAll',
        '2 label:View',
        '2.0 role:toggleDevTools',
        '2.2 role:resetZoom',
        '2.6 role:togglefullscreen',
        '3 label:Window',
        '3.0 role:minimize',
        '3.4 role:front',
      ]),
    )
    expect(
      shortcutItems
        .filter(({ item }) => item.registerAccelerator !== false)
        .map(describeMenuItem),
    ).toEqual([])
  })
})

type MenuItemAtPath = {
  item: MenuItemConstructorOptions
  path: string
}

function collectShortcutMenuItems(
  items: MenuItemConstructorOptions[],
  parentPath = '',
): MenuItemAtPath[] {
  return items.flatMap((item, index) => {
    const path = parentPath ? `${parentPath}.${index}` : String(index)
    const current = item.type === 'separator' ? [] : [{ item, path }]
    const submenu = Array.isArray(item.submenu)
      ? collectShortcutMenuItems(item.submenu, path)
      : []
    return [...current, ...submenu]
  })
}

function describeMenuItem({ item, path }: MenuItemAtPath) {
  if (item.role) return `${path} role:${item.role}`
  if (item.label) return `${path} label:${item.label}`
  if (item.type) return `${path} type:${item.type}`
  return `${path} ${JSON.stringify(item)}`
}
