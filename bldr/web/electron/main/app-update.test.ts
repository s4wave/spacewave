import { spawn, execFile } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { startAppBundleUpdate } from './app-update.js'

vi.mock('node:child_process', async (importOriginal) => {
  const { EventEmitter } = await import('node:events')
  const original = await importOriginal<typeof import('node:child_process')>()
  const execFile = vi.fn((...args: unknown[]) => {
    const callback = args.at(-1) as (
      err: Error | null,
      stdout: string,
      stderr: string,
    ) => void
    queueMicrotask(() => callback(null, '', ''))
  })
  const spawn = vi.fn(() => {
    const child = Object.assign(new EventEmitter(), { unref: vi.fn() })
    queueMicrotask(() => child.emit('spawn'))
    return child
  })
  return {
    ...original,
    execFile,
    spawn,
    default: { ...original, execFile, spawn },
  }
})

const fixtures: string[] = []

afterEach(async () => {
  vi.clearAllMocks()
  await Promise.all(
    fixtures.splice(0).map((dir) => rm(dir, { recursive: true, force: true })),
  )
})

describe('startAppBundleUpdate', () => {
  it('copies the helper outside both app bundles and waits for the Electron PID', async () => {
    // Put both app copies and helper state under this checkout's disposable root.
    const root = path.join(process.cwd(), '.tmp', 'daemon-phase4-targets')
    await mkdir(root, { recursive: true })
    const fixture = await mkdtemp(path.join(root, 'app-copy-'))
    fixtures.push(fixture)
    const currentApp = path.join(fixture, 'Installed.app')
    const executable = path.join(currentApp, 'Contents', 'MacOS', 'spacewave')
    const stagedApp = path.join(fixture, 'Staged.app')
    const stagedHelper = path.join(
      stagedApp,
      'Contents',
      'MacOS',
      'spacewave-helper',
    )
    await mkdir(path.dirname(executable), { recursive: true })
    await mkdir(path.dirname(stagedHelper), { recursive: true })
    await writeFile(executable, 'old app')
    await writeFile(stagedHelper, 'verified helper')

    // Launch the verified helper using only the app process's own identity.
    const stateDir = path.join(fixture, 'state')
    await startAppBundleUpdate(stagedApp, executable, stateDir, 4242)

    const [helperPath, args, options] = vi.mocked(spawn).mock.calls[0]
    expect(await readFile(helperPath, 'utf8')).toBe('verified helper')
    expect(args).toEqual([
      '--update',
      '--current',
      currentApp,
      '--staged',
      stagedApp,
      '--pid',
      '4242',
      '--pipe-root',
      stateDir,
      '--pipe-id',
      'update',
    ])
    expect(options).toEqual({ detached: true, stdio: 'ignore' })
    expect(vi.mocked(execFile)).toHaveBeenCalledWith(
      'codesign',
      ['--verify', '--deep', '--strict', stagedApp],
      expect.any(Function),
    )
    expect(helperPath.startsWith(stateDir + path.sep)).toBe(true)
  })

  it('rejects a process outside an app bundle before launching a helper', async () => {
    await expect(
      startAppBundleUpdate(
        '/tmp/Staged.app',
        '/tmp/spacewave',
        '/tmp/state',
        1,
      ),
    ).rejects.toThrow('not running from an installed .app')
    expect(spawn).not.toHaveBeenCalled()
  })
})
