// Global test setup for vitest
// Mocks localStorage for tests that use state persistence, and unmounts every
// rendered tree after each test.

import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = value
    },
    removeItem: (key: string) => {
      delete store[key]
    },
    clear: () => {
      store = {}
    },
    get length() {
      return Object.keys(store).length
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
  }
})()

Object.defineProperty(globalThis, 'localStorage', {
  value: localStorageMock,
  writable: true,
})

// Testing Library registers its own cleanup only when vitest's globals are on,
// and they are off here. A tree left mounted keeps rendering as its pending
// work settles, and a render scheduled after the file's last test runs once
// happy-dom has torn down, failing the run with "window is not defined".
afterEach(cleanup)
