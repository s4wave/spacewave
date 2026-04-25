/**
 * Browser-specific test setup for vitest browser mode (Playwright).
 *
 * Unlike setup.ts (used for unit tests with happy-dom), this file does NOT
 * mock localStorage since the real browser provides a working localStorage.
 *
 * This file handles:
 * - Importing app CSS so e2e tests don't need to import it individually
 * - Forcing dark mode color scheme
 */

// Import app CSS so individual e2e test files don't need to
import '@s4wave/web/style/app.css'

// Force dark mode for consistent test rendering
document.documentElement.classList.add('dark')
