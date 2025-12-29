import { spawn, ChildProcess, execSync } from 'child_process'
import { setTimeout } from 'timers/promises'

const SERVER_PORT = 8091

let serverProcess: ChildProcess | null = null

export async function setup() {
  console.log('[Setup] Starting Go server for QuickJS browser tests...')

  // Kill any existing process on the port
  try {
    execSync(`lsof -ti:${SERVER_PORT} | xargs kill -9 2>/dev/null || true`, {
      stdio: 'ignore',
    })
    await setTimeout(200)
  } catch {
    // Ignore errors
  }

  serverProcess = spawn('go', ['run', 'main.go'], {
    cwd: import.meta.dirname,
    env: { ...process.env, PORT: String(SERVER_PORT) },
    stdio: ['pipe', 'pipe', 'pipe'],
  })

  // Wait for server to be ready
  let ready = false
  const timeout = Date.now() + 15000

  serverProcess.stdout?.on('data', (data: Buffer) => {
    const output = data.toString()
    console.log('[Go Server]', output.trim())
    if (output.includes('Server running')) {
      ready = true
    }
  })

  serverProcess.stderr?.on('data', (data: Buffer) => {
    console.error('[Go Server Error]', data.toString().trim())
  })

  serverProcess.on('error', (err) => {
    console.error('[Go Server] Failed to start:', err)
  })

  // Poll until ready or timeout
  while (!ready && Date.now() < timeout) {
    await setTimeout(100)
  }

  if (!ready) {
    serverProcess?.kill('SIGTERM')
    throw new Error('Go server did not start in time')
  }

  console.log('[Setup] Go server is ready')
}

export async function teardown() {
  if (serverProcess) {
    console.log('[Teardown] Stopping Go server...')
    serverProcess.kill('SIGKILL')
    serverProcess = null
  }
  // Also kill anything still on the port
  try {
    execSync(`lsof -ti:${SERVER_PORT} | xargs kill -9 2>/dev/null || true`, {
      stdio: 'ignore',
    })
  } catch {
    // Ignore
  }
}
