import assert from 'node:assert/strict'
import { spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, rmSync, symlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'
import { parentPort, Worker, type MessagePort } from 'node:worker_threads'

import { acquireDirectoryLock } from './lock.js'

// Child Workers and child processes re-enter this bundled file with a child
// mode in the environment, so the lock is exercised from separate thread and
// OS process contexts instead of the test runner's own connection.
const childMode = process.env.SPACEWAVE_LOCK_TEST_CHILD

function childPort(): MessagePort {
  if (!parentPort) throw new Error('worker child mode requires a Worker parent')
  return parentPort
}

function childDir(): string {
  const dir = process.env.SPACEWAVE_LOCK_TEST_DIR
  if (!dir) throw new Error('missing SPACEWAVE_LOCK_TEST_DIR')
  return dir
}

if (childMode === 'worker-hold') {
  const lock = acquireDirectoryLock(childDir())
  childPort().postMessage({ acquired: true })
  childPort().once('message', () => lock.close())
} else if (childMode === 'worker-try') {
  try {
    acquireDirectoryLock(childDir()).close()
    childPort().postMessage({ acquired: true })
  } catch (err) {
    childPort().postMessage({
      acquired: false,
      message: err instanceof Error ? err.message : String(err),
    })
  }
} else if (childMode === 'process-hold') {
  acquireDirectoryLock(childDir())
  console.log('ACQUIRED')
  // Stay alive so the parent can SIGKILL this process while it holds the lock.
  setInterval(() => {}, 1_000_000)
} else {
  runTests()
}

function runTests(): void {
  // bundlePath is this emitted test bundle. Children run it as their entry.
  const bundlePath = fileURLToPath(import.meta.url)

  function spawnWorker(mode: string, dir: string): Worker {
    return new Worker(bundlePath, {
      env: {
        ...process.env,
        SPACEWAVE_LOCK_TEST_CHILD: mode,
        SPACEWAVE_LOCK_TEST_DIR: dir,
      },
    })
  }

  function workerMessage(
    worker: Worker,
  ): Promise<{ acquired: boolean; message?: string }> {
    return new Promise((resolve, reject) => {
      worker.once('message', resolve)
      worker.once('error', reject)
    })
  }

  function workerExit(worker: Worker): Promise<void> {
    return new Promise((resolve) => worker.once('exit', () => resolve()))
  }

  function childExit(child: ChildProcess): Promise<void> {
    return new Promise((resolve) => child.once('exit', () => resolve()))
  }

  function stdoutMarker(child: ChildProcess, marker: string): Promise<void> {
    const stdout = child.stdout
    if (!stdout) throw new Error('child stdout must be piped')
    return new Promise((resolve, reject) => {
      let seen = ''
      stdout.on('data', (chunk: Buffer) => {
        seen += chunk.toString()
        if (seen.includes(marker)) resolve()
      })
      child.once('error', reject)
      child.once('exit', (code) =>
        reject(new Error(`child exited before ${marker}: code ${code}`)),
      )
    })
  }

  test('denies a second Worker for the same physical directory', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'spacewave-lock-'))
    let holder: Worker | undefined
    try {
      holder = spawnWorker('worker-hold', dir)
      assert.deepEqual(await workerMessage(holder), { acquired: true })
      const denied = await workerMessage(spawnWorker('worker-try', dir))
      assert.equal(denied.acquired, false)
      assert.match(denied.message ?? '', /database is locked/)
      holder.postMessage('release')
      await workerExit(holder)
    } finally {
      void holder?.terminate()
      rmSync(dir, { recursive: true, force: true })
    }
  })

  test('denies a second Worker through a symlink alias of the same directory', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'spacewave-lock-'))
    const alias = `${dir}-alias`
    symlinkSync(dir, alias)
    let holder: Worker | undefined
    try {
      holder = spawnWorker('worker-hold', dir)
      assert.deepEqual(await workerMessage(holder), { acquired: true })
      const denied = await workerMessage(spawnWorker('worker-try', alias))
      assert.equal(denied.acquired, false)
      holder.postMessage('release')
      await workerExit(holder)
    } finally {
      void holder?.terminate()
      rmSync(alias, { recursive: true, force: true })
      rmSync(dir, { recursive: true, force: true })
    }
  })

  test('reopens after a normal close', () => {
    const dir = mkdtempSync(join(tmpdir(), 'spacewave-lock-'))
    try {
      const first = acquireDirectoryLock(dir)
      first.close()
      first.close()
      const second = acquireDirectoryLock(dir)
      assert.equal(second.directory, first.directory)
      second.close()
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  test('reopens after abrupt owner termination', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'spacewave-lock-'))
    let child: ChildProcess | undefined
    try {
      child = spawn(process.execPath, [bundlePath], {
        env: {
          ...process.env,
          SPACEWAVE_LOCK_TEST_CHILD: 'process-hold',
          SPACEWAVE_LOCK_TEST_DIR: dir,
        },
        stdio: ['ignore', 'pipe', 'inherit'],
      })
      await stdoutMarker(child, 'ACQUIRED')
      child.kill('SIGKILL')
      await childExit(child)
      acquireDirectoryLock(dir).close()
    } finally {
      child?.kill('SIGKILL')
      rmSync(dir, { recursive: true, force: true })
    }
  })

  test('independent directory locks coexist', () => {
    const dirA = mkdtempSync(join(tmpdir(), 'spacewave-lock-a-'))
    const dirB = mkdtempSync(join(tmpdir(), 'spacewave-lock-b-'))
    try {
      const a = acquireDirectoryLock(dirA)
      const b = acquireDirectoryLock(dirB)
      assert.notEqual(a.directory, b.directory)
      a.close()
      b.close()
    } finally {
      rmSync(dirA, { recursive: true, force: true })
      rmSync(dirB, { recursive: true, force: true })
    }
  })
}
