import { afterEach, describe, expect, it } from 'vitest'

import {
  clearObsoleteShellTabsState,
  readShellDocumentState,
  removeObsoleteShellTabState,
  writeShellDocumentState,
} from './ShellDocumentState.js'

function restoreStorage(
  name: 'localStorage' | 'sessionStorage',
  descriptor: PropertyDescriptor | undefined,
): void {
  if (descriptor) {
    Object.defineProperty(globalThis, name, descriptor)
  } else {
    delete (globalThis as Record<string, unknown>)[name]
  }
}

describe('ShellDocumentState storage capabilities', () => {
  afterEach(() => {
    localStorage.clear()
    sessionStorage.clear()
  })

  it('persists a committed-create selection handoff', () => {
    writeShellDocumentState({
      incarnation: 'document-b',
      activeTabId: 'existing',
      pendingCreatedTabId: 'created',
    })

    expect(readShellDocumentState()).toEqual({
      incarnation: 'document-b',
      activeTabId: 'existing',
      pendingCreatedTabId: 'created',
    })
  })

  it('treats storage getter failures as unavailable best-effort cleanup', () => {
    const previousLocalStorage = Object.getOwnPropertyDescriptor(
      globalThis,
      'localStorage',
    )
    const previousSessionStorage = Object.getOwnPropertyDescriptor(
      globalThis,
      'sessionStorage',
    )
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      get: () => {
        throw new Error('opaque local storage')
      },
    })
    Object.defineProperty(globalThis, 'sessionStorage', {
      configurable: true,
      get: () => {
        throw new Error('opaque session storage')
      },
    })

    try {
      expect(() => removeObsoleteShellTabState('tab-1')).not.toThrow()
      expect(() => clearObsoleteShellTabsState()).not.toThrow()
      expect(readShellDocumentState()).toBeNull()
      expect(() =>
        writeShellDocumentState({ incarnation: 'doc-1', activeTabId: 'tab-1' }),
      ).not.toThrow()
    } finally {
      restoreStorage('localStorage', previousLocalStorage)
      restoreStorage('sessionStorage', previousSessionStorage)
    }
  })
})
