import { afterEach, describe, expect, test } from 'bun:test'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import {
  goScriptStorageBenchmarkEngines,
  parseGoScriptStorageBenchmarkArgs,
  runGoScriptStorageBenchmarkMatrix,
  validateGoScriptStorageBenchmarkIndex,
  type GoScriptStorageBenchmarkEngine,
  type GoScriptStorageBenchmarkOptions,
} from './goscript-storage-benchmark.js'

const roots: string[] = []

afterEach(async () => {
  await Promise.all(
    roots.splice(0).map((root) => rm(root, { force: true, recursive: true })),
  )
})

describe('GoScript storage benchmark matrix', () => {
  test('parses one safe run identity', () => {
    const parsed = parseGoScriptStorageBenchmarkArgs(
      ['--run-id', 'run-20260804', '--chromium-cpu-profile'],
      '/repo',
    )
    expect(parsed).toEqual({
      runId: 'run-20260804',
      outputRoot: '/repo/.tmp/e2e/goscriptbench',
      chromiumCpuProfile: true,
      engines: [...goScriptStorageBenchmarkEngines],
    })
    expect(() =>
      parseGoScriptStorageBenchmarkArgs(['--run-id', '../escape'], '/repo'),
    ).toThrow('safe artifact path component')
    expect(
      parseGoScriptStorageBenchmarkArgs(
        [
          '--run-id',
          'run-selected',
          '--engine',
          'webkit',
          '--engine',
          'chromium',
        ],
        '/repo',
      ).engines,
    ).toEqual(['chromium', 'webkit'])
    expect(() =>
      parseGoScriptStorageBenchmarkArgs(
        [
          '--run-id',
          'run-duplicate',
          '--engine',
          'chromium',
          '--engine',
          'chromium',
        ],
        '/repo',
      ),
    ).toThrow('selected more than once')
  })

  test('runs every engine sequentially and keeps failures local', async () => {
    const root = await mkdtemp(join(tmpdir(), 'goscript-storage-matrix-'))
    roots.push(root)
    const options: GoScriptStorageBenchmarkOptions = {
      runId: 'run-20260804',
      outputRoot: root,
      chromiumCpuProfile: false,
      engines: [...goScriptStorageBenchmarkEngines],
      repoRoot: '/repo',
      spacewaveRevision: 'spacewave-revision',
      goScriptRevision: 'goscript-revision',
    }
    const order: GoScriptStorageBenchmarkEngine[] = []
    let active = false
    const index = await runGoScriptStorageBenchmarkMatrix(
      options,
      async ({ engine }) => {
        expect(active).toBeFalse()
        active = true
        order.push(engine)
        await Promise.resolve()
        active = false
        if (engine === 'firefox') {
          throw new Error('Firefox failed')
        }
        return {
          engine,
          status: 'passed',
          exitCode: 0,
          engineVersion: `${engine}-version`,
          fixtureSha256: 'fixture-sha256',
          reason: '',
          artifactDir: engine,
          logFile: `logs/${engine}.log`,
        }
      },
    )

    expect(order).toEqual([...goScriptStorageBenchmarkEngines])
    expect(index.engines.map((engine) => engine.status)).toEqual([
      'passed',
      'failed',
      'passed',
    ])
    expect('medianDisplayReadyMs' in index).toBeFalse()
    expect('p95DisplayReadyMs' in index).toBeFalse()
    expect(
      await Bun.file(join(root, options.runId, 'index.json')).json(),
    ).toEqual(index)
  })

  test('keeps an unsupported engine outside the measured fixture set', async () => {
    const root = await mkdtemp(join(tmpdir(), 'goscript-storage-matrix-'))
    roots.push(root)
    const options: GoScriptStorageBenchmarkOptions = {
      runId: 'run-unsupported',
      outputRoot: root,
      chromiumCpuProfile: false,
      engines: ['chromium', 'webkit'],
      repoRoot: '/repo',
      spacewaveRevision: 'spacewave-revision',
      goScriptRevision: 'goscript-revision',
    }
    const index = await runGoScriptStorageBenchmarkMatrix(
      options,
      async ({ engine }) =>
        engine === 'webkit'
          ? {
              engine,
              status: 'unsupported',
              exitCode: 0,
              engineVersion: '26.5',
              fixtureSha256: '',
              reason: 'navigator.storage.getDirectory is unavailable',
              artifactDir: engine,
              logFile: `logs/${engine}.log`,
            }
          : {
              engine,
              status: 'passed',
              exitCode: 0,
              engineVersion: `${engine}-version`,
              fixtureSha256: 'fixture-sha256',
              reason: '',
              artifactDir: engine,
              logFile: `logs/${engine}.log`,
            },
    )

    expect(index.fixtureSha256).toBe('fixture-sha256')
    expect(index.engines.map((engine) => engine.status)).toEqual([
      'passed',
      'unsupported',
    ])
    expect(index.engines[1]?.fixtureSha256).toBe('')
  })

  test('rejects unlike fixture identities', () => {
    const index = {
      schemaVersion: 1 as const,
      runId: 'run-20260804',
      spacewaveRevision: 'spacewave-revision',
      goScriptRevision: 'goscript-revision',
      fixtureSha256: 'fixture-a',
      engines: goScriptStorageBenchmarkEngines.map((engine) => ({
        engine,
        status: 'passed' as const,
        exitCode: 0,
        engineVersion: `${engine}-version`,
        fixtureSha256: engine === 'webkit' ? 'fixture-b' : 'fixture-a',
        reason: '',
        artifactDir: engine,
        logFile: `logs/${engine}.log`,
      })),
    }
    expect(() => validateGoScriptStorageBenchmarkIndex(index)).toThrow(
      'webkit fixture differs from the matrix',
    )
  })
})
