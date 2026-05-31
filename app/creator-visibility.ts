import { useSyncExternalStore } from 'react'

export const EXPERIMENTAL_CREATORS_STORAGE_KEY =
  'spacewave-show-experimental-creators'

const experimentalCreatorsChangeEvent = 'spacewave:experimental-creators-change'

function readExperimentalCreatorsStorage(): boolean {
  if (typeof localStorage === 'undefined') return false
  try {
    return parseExperimentalCreatorsStorageValue(
      localStorage.getItem(EXPERIMENTAL_CREATORS_STORAGE_KEY),
    )
  } catch {
    return false
  }
}

function parseExperimentalCreatorsStorageValue(value: string | null): boolean {
  if (!value) return false
  switch (value.trim().toLowerCase()) {
    case '1':
    case 'true':
    case 'yes':
    case 'on':
      return true
    default:
      return false
  }
}

function notifyExperimentalCreatorsChanged(): void {
  if (typeof dispatchEvent !== 'function') return
  dispatchEvent(new Event(experimentalCreatorsChangeEvent))
}

function subscribeExperimentalCreatorsChanged(
  callback: () => void,
): () => void {
  if (typeof addEventListener !== 'function') return () => undefined

  const handleStorage = (storageEvent: StorageEvent) => {
    if (
      storageEvent.key &&
      storageEvent.key !== EXPERIMENTAL_CREATORS_STORAGE_KEY
    ) {
      return
    }
    callback()
  }
  addEventListener('storage', handleStorage)
  addEventListener(experimentalCreatorsChangeEvent, callback)
  return () => {
    removeEventListener('storage', handleStorage)
    removeEventListener(experimentalCreatorsChangeEvent, callback)
  }
}

// areExperimentalCreatorsEnabled returns true when the current browser should
// expose experimental creator affordances.
export function areExperimentalCreatorsEnabled(
  isDev = !!import.meta.env?.DEV,
  preferenceEnabled = readExperimentalCreatorsStorage(),
): boolean {
  return isDev || preferenceEnabled
}

export function setExperimentalCreatorsEnabled(enabled: boolean): void {
  if (typeof localStorage === 'undefined') return
  try {
    if (enabled) {
      localStorage.setItem(EXPERIMENTAL_CREATORS_STORAGE_KEY, '1')
      return
    }
    localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
  } finally {
    notifyExperimentalCreatorsChanged()
  }
}

export function useExperimentalCreatorsEnabled(
  isDev = !!import.meta.env?.DEV,
): boolean {
  return useSyncExternalStore(
    subscribeExperimentalCreatorsChanged,
    () => areExperimentalCreatorsEnabled(isDev),
    () => isDev,
  )
}

// isExperimentalCreatorVisible returns true when a creator should be shown for
// the current browser.
export function isExperimentalCreatorVisible(
  experimental: boolean | undefined,
  experimentalCreatorsEnabled = areExperimentalCreatorsEnabled(),
): boolean {
  return !(experimental ?? false) || experimentalCreatorsEnabled
}
