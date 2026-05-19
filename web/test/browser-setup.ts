/**
 * Browser-specific test setup for vitest browser mode (Playwright).
 *
 * Unlike setup.ts (used for unit tests with happy-dom), this file does NOT
 * mock localStorage since the real browser provides a working localStorage.
 *
 * This file handles:
 * - Importing app CSS so e2e tests don't need to import it individually
 * - Forcing dark mode color scheme
 * - Resetting shared browser page state between E2E tests
 */
import { afterEach, beforeEach } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup } from 'vitest-browser-react'

// Import app CSS so individual e2e test files don't need to
import '@s4wave/web/style/app.css'

// Force dark mode for consistent test rendering
document.documentElement.classList.add('dark')

async function resetBrowserTestState() {
  await cleanup()
  await page.viewport(1280, 800)
  localStorage.clear()
  window.location.hash = ''
}

beforeEach(resetBrowserTestState)
afterEach(resetBrowserTestState)
