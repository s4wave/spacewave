/**
 * Shared test helpers for reducing boilerplate across test files.
 *
 * Provides common wrapper components and render functions used in 3+ tests.
 */
import React from 'react'
import { AppShell } from '@s4wave/app/AppShell.js'
import type { SetBottomBarOpenMenu } from '../frame/bottom-bar-context.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'

// Re-export common testing utilities for convenience
export { cleanup, render } from '@testing-library/react'
export { vi, describe, it, expect, beforeEach, afterEach } from 'vitest'

/**
 * TestWrapper wraps children in a minimal shared test shell.
 */
export function TestWrapper({ children }: { children: React.ReactNode }) {
  return <>{children}</>
}

/**
 * ShellWrapper wraps children in AppShell.
 *
 * Used by e2e tests that render EditorShell or other full-shell components.
 * This is the most common wrapper in e2e tests (App.e2e.test,
 * TabDragOverlay.e2e.test, ShellFlexLayout.e2e.test, etc).
 */
export function ShellWrapper({ children }: { children: React.ReactNode }) {
  return <AppShell>{children}</AppShell>
}

/**
 * BottomBarWrapper wraps children in BottomBarRoot with mock handlers.
 *
 * Used by tests that need the bottom bar context (SessionFrame.test, etc).
 */
export function BottomBarWrapper({
  children,
  openMenu = '',
  setOpenMenu,
}: {
  children: React.ReactNode
  openMenu?: string
  setOpenMenu?: SetBottomBarOpenMenu
}) {
  return (
    <BottomBarRoot setOpenMenu={setOpenMenu ?? (() => {})} openMenu={openMenu}>
      {children}
    </BottomBarRoot>
  )
}

/**
 * cleanupBrowserTest provides common beforeEach setup for e2e tests that
 * clears state between tests.
 *
 * Clears localStorage, resets URL hash, and runs cleanup.
 * This pattern is repeated in App.e2e.test, ShellFlexLayout.e2e.test,
 * TabDragOverlay.e2e.test, PrerenderedApp.e2e.test, and UserStory.e2e.test.
 */
export function cleanupBrowserTest(cleanup: () => void | Promise<void>) {
  void cleanup()
  localStorage.clear()
  window.location.hash = ''
}
