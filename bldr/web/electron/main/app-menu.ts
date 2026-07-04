import type { MenuItemConstructorOptions } from 'electron'

export interface ApplicationMenuOptions {
  appName: string
  isDebug: boolean
  isMac: boolean
}

// buildApplicationMenuTemplate keeps native menu clicks while leaving every
// in-app shortcut to the renderer-owned keybinding dispatcher.
export function buildApplicationMenuTemplate({
  appName,
  isDebug,
  isMac,
}: ApplicationMenuOptions): MenuItemConstructorOptions[] {
  return withoutRegisteredAccelerators([
    ...(isMac ? [buildMacAppMenu(appName)] : []),
    buildEditMenu(),
    buildViewMenu(isDebug),
    buildWindowMenu(isMac),
  ])
}

function buildMacAppMenu(appName: string): MenuItemConstructorOptions {
  return {
    label: appName,
    submenu: [
      { role: 'about' },
      { type: 'separator' },
      { role: 'services' },
      { type: 'separator' },
      { role: 'hide' },
      { role: 'hideOthers' },
      { role: 'unhide' },
      { type: 'separator' },
      { role: 'quit' },
    ],
  }
}

function buildEditMenu(): MenuItemConstructorOptions {
  return {
    label: 'Edit',
    submenu: [
      { role: 'undo' },
      { role: 'redo' },
      { type: 'separator' },
      { role: 'cut' },
      { role: 'copy' },
      { role: 'paste' },
      { role: 'pasteAndMatchStyle' },
      { role: 'delete' },
      { type: 'separator' },
      { role: 'selectAll' },
    ],
  }
}

function buildViewMenu(isDebug: boolean): MenuItemConstructorOptions {
  return {
    label: 'View',
    submenu: [
      ...(isDebug
        ? [
            { role: 'toggleDevTools' as const },
            { type: 'separator' as const },
          ]
        : []),
      { role: 'resetZoom' },
      { role: 'zoomIn' },
      { role: 'zoomOut' },
      { type: 'separator' },
      { role: 'togglefullscreen' },
    ],
  }
}

function buildWindowMenu(isMac: boolean): MenuItemConstructorOptions {
  return {
    label: 'Window',
    submenu: [
      { role: 'minimize' },
      { role: 'zoom' },
      { role: 'close' },
      ...(isMac
        ? [
            { type: 'separator' as const },
            { role: 'front' as const },
          ]
        : []),
    ],
  }
}

function withoutRegisteredAccelerators(
  items: MenuItemConstructorOptions[],
): MenuItemConstructorOptions[] {
  return items.map((item) => {
    const next: MenuItemConstructorOptions = {
      ...item,
      registerAccelerator: false,
    }
    if (Array.isArray(item.submenu)) {
      next.submenu = withoutRegisteredAccelerators(item.submenu)
    }
    return next
  })
}
