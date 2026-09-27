import { afterEach, expect, test } from 'bun:test'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { createInterface } from 'node:readline'
import path from 'node:path'

const electronBinary = process.env['E2E_ELECTRON_BINARY']
const stateRoot = process.env['SPACEWAVE_STATE_PATH']
const childScript = path.join(import.meta.dir, 'profile-lock-child.cjs')
const children: ChildProcessWithoutNullStreams[] = []

afterEach(() => {
  for (const child of children.splice(0)) {
    child.kill()
  }
})

/** launchProfile observes Electron's lock and window events through stdout. */
function launchProfile(profile: string) {
  mkdirSync(profile, { recursive: true })
  const child = spawn(electronBinary!, [childScript], {
    env: { ...process.env, BLDR_PLUGIN_STATE_PATH: profile },
    stdio: ['pipe', 'pipe', 'pipe'],
  })
  children.push(child)

  const events: string[] = []
  let stderr = ''
  const listeners = new Set<(event: string) => void>()
  child.stderr.setEncoding('utf8').on('data', (chunk: string) => {
    stderr += chunk
  })
  createInterface({ input: child.stdout }).on('line', (event) => {
    events.push(event)
    for (const listener of listeners) listener(event)
  })

  return {
    child,
    events,
    next(expected: string): Promise<void> {
      if (events.includes(expected)) return Promise.resolve()
      return new Promise((resolve, reject) => {
        const listener = (event: string) => {
          if (event !== expected) return
          listeners.delete(listener)
          child.off('exit', onExit)
          resolve()
        }
        const onExit = (code: number | null, signal: string | null) => {
          listeners.delete(listener)
          reject(
            new Error(
              `Electron exited before ${expected}: code=${code} signal=${signal}\n${stderr}`,
            ),
          )
        }
        listeners.add(listener)
        child.once('exit', onExit)
      })
    },
  }
}

test.skipIf(!electronBinary)(
  'two Electron profiles retain independent single-instance locks',
  async () => {
    if (!stateRoot) {
      throw new Error(
        'SPACEWAVE_STATE_PATH is required for the Electron profile-lock fixture',
      )
    }

    const rootA = path.join(stateRoot, 'a')
    const rootB = path.join(stateRoot, 'b')
    const a = launchProfile(rootA)
    await a.next('ready')
    const b = launchProfile(rootB)
    await b.next('ready')

    const secondA = launchProfile(rootA)
    await secondA.next('denied')
    await a.next('focused')
    expect(b.events).toEqual(['ready'])

    const aExited = new Promise<void>((resolve) =>
      a.child.once('exit', () => resolve()),
    )
    a.child.stdin.write('quit\n')
    await aExited
    expect(b.child.exitCode).toBeNull()

    const secondB = launchProfile(rootB)
    await secondB.next('denied')
    await b.next('focused')
  },
  30_000,
)
