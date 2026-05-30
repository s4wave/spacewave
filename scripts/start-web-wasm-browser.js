#!/usr/bin/env bun
/* eslint-disable react-doctor/async-await-in-loop */

// start-web-wasm-browser.js - Launch a web dev script and open Playwright Chromium with dark mode.
// Persistent browser state is stored in .bldr/browser-state/<profile>.

import { spawn } from 'child_process'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'
import { mkdir } from 'fs/promises'
import { chromium } from 'playwright'

const __dirname = dirname(fileURLToPath(import.meta.url))
const rootDir = join(__dirname, '..')
const startScript = process.env.SPACEWAVE_WEB_START_SCRIPT || 'start:web:wasm'
const browserProfile = process.env.SPACEWAVE_WEB_BROWSER_PROFILE || 'playwright'
const userDataDir = join(rootDir, '.bldr', 'browser-state', browserProfile)
const serverUrl = process.env.SPACEWAVE_WEB_SERVER_URL || 'http://127.0.0.1:8080'
const readyPath = process.env.SPACEWAVE_WEB_READY_PATH || '/entrypoint/runtime.wasm'
const readyUrl = new URL(readyPath, serverUrl).toString()

async function waitForServer(timeout = 120000) {
  const start = Date.now()
  // First wait for the server to respond at all
  console.log('Waiting for server to start...')
  while (Date.now() - start < timeout) {
    try {
      const res = await fetch(serverUrl)
      if (res.ok) break
    } catch {
      // Server not ready yet
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  if (Date.now() - start >= timeout) {
    throw new Error(`Server did not start within ${timeout}ms`)
  }
  console.log(`Server responding, waiting for runtime at ${readyUrl}...`)
  // Now wait for the runtime artifact to be built.
  while (Date.now() - start < timeout) {
    try {
      const res = await fetch(readyUrl, { method: 'HEAD' })
      if (res.ok) return true
    } catch {
      // Runtime not ready yet
    }
    await new Promise((r) => setTimeout(r, 1000))
  }
  throw new Error(`Runtime build did not complete within ${timeout}ms`)
}

async function main() {
  // Ensure user data directory exists
  await mkdir(userDataDir, { recursive: true })

  // Start the web server
  console.log(`Starting web server with bun run ${startScript}...`)
  const server = spawn('bun', ['run', startScript], {
    cwd: rootDir,
    stdio: ['inherit', 'pipe', 'pipe'],
    detached: false,
  })

  // Forward server output
  server.stdout.pipe(process.stdout)
  server.stderr.pipe(process.stderr)

  // Handle server errors
  server.on('error', (err) => {
    console.error('Failed to start server:', err)
    process.exit(1)
  })

  // Wait for server and runtime artifact to be ready.
  try {
    await waitForServer()
  } catch (err) {
    console.error(err.message)
    server.kill()
    process.exit(1)
  }
  console.log('Server and runtime build ready.')

  // Launch Playwright with persistent context
  console.log('Launching browser...')
  const context = await chromium.launchPersistentContext(userDataDir, {
    headless: false,
    colorScheme: 'dark',
    viewport: null, // Dynamic viewport - resizes with window
    // args: ['--disable-web-security'], // Allow cross-origin for local dev
  })

  const page = context.pages()[0] || (await context.newPage())
  await page.goto(serverUrl)
  console.log('Browser opened. Close the browser window to exit.')

  // Wait for browser to close
  await new Promise((resolve) => {
    context.on('close', resolve)
  })

  // Clean up server
  console.log('Browser closed. Shutting down server...')
  server.kill('SIGTERM')

  // Give server a moment to clean up, then force kill if needed
  await new Promise((r) => setTimeout(r, 1000))
  if (!server.killed) {
    server.kill('SIGKILL')
  }

  process.exit(0)
}

main().catch((err) => {
  console.error('Error:', err)
  process.exit(1)
})
