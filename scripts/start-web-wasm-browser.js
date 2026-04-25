#!/usr/bin/env bun

// start-web-wasm-browser.js - Launch start:web:wasm and open Playwright Chromium with dark mode.
// Persistent browser state is stored in .bldr/browser-state/playwright

import { spawn } from 'child_process'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'
import { mkdir } from 'fs/promises'
import { chromium } from 'playwright'

const __dirname = dirname(fileURLToPath(import.meta.url))
const rootDir = join(__dirname, '..')
const userDataDir = join(rootDir, '.bldr', 'browser-state', 'playwright')
const serverUrl = 'http://127.0.0.1:8080'
const wasmUrl = `${serverUrl}/entrypoint/runtime.wasm`

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
  console.log('Server responding, waiting for WASM build...')
  // Now wait for the WASM file to be built
  while (Date.now() - start < timeout) {
    try {
      const res = await fetch(wasmUrl, { method: 'HEAD' })
      if (res.ok) return true
    } catch {
      // WASM not ready yet
    }
    await new Promise((r) => setTimeout(r, 1000))
  }
  throw new Error(`WASM build did not complete within ${timeout}ms`)
}

async function main() {
  // Ensure user data directory exists
  await mkdir(userDataDir, { recursive: true })

  // Start the web server
  console.log('Starting web server...')
  const server = spawn('bun', ['run', 'start:web:wasm'], {
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

  // Wait for server and WASM to be ready
  try {
    await waitForServer()
  } catch (err) {
    console.error(err.message)
    server.kill()
    process.exit(1)
  }
  console.log('Server and WASM build ready.')

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
