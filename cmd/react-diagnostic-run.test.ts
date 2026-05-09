import { describe, expect, it } from 'vitest'

import {
  buildReactDiagnosticRunReport,
  buildReactDoctorArgs,
  parseReactDiagnosticRunArgs,
} from './react-diagnostic-run.js'

describe('react diagnostic run wrapper', () => {
  it('defaults to scanning the current directory offline', () => {
    const options = parseReactDiagnosticRunArgs([])

    expect(options).toEqual({
      directory: '.',
      outputPath: null,
      offline: true,
      compact: false,
      passthroughArgs: [],
    })
    expect(buildReactDoctorArgs(options)).toEqual([
      '.',
      '--json',
      '--offline',
      '--fail-on',
      'none',
    ])
  })

  it('keeps wrapper flags out of the React Doctor argument list', () => {
    const options = parseReactDiagnosticRunArgs([
      'app',
      '--online',
      '--output',
      '.tmp/react-diagnostic.json',
      '--compact',
      '--project',
      'spacewave',
      '--diff',
      'master',
    ])

    expect(options).toEqual({
      directory: 'app',
      outputPath: '.tmp/react-diagnostic.json',
      offline: false,
      compact: true,
      passthroughArgs: ['--project', 'spacewave', '--diff', 'master'],
    })
    expect(buildReactDoctorArgs(options)).toEqual([
      'app',
      '--json',
      '--fail-on',
      'none',
      '--project',
      'spacewave',
      '--diff',
      'master',
    ])
  })

  it('does not override an explicit React Doctor fail-on level', () => {
    const options = parseReactDiagnosticRunArgs(['--fail-on', 'warning'])

    expect(buildReactDoctorArgs(options)).toEqual([
      '.',
      '--json',
      '--offline',
      '--fail-on',
      'warning',
    ])
  })

  it('wraps the React Doctor report in a schema-versioned run report', () => {
    const report = buildReactDiagnosticRunReport({
      directory: '.',
      offline: true,
      args: ['.', '--json', '--offline'],
      generatedAt: '2026-05-09T00:00:00.000Z',
      processResult: {
        code: 0,
        signal: null,
        stdout: '{}',
        stderr: '',
      },
      reactDoctor: {
        schemaVersion: 1,
        ok: true,
        directory: '/repo',
        mode: 'full',
        diagnostics: [],
        summary: { totalDiagnosticCount: 0 },
        error: null,
      },
      parseError: null,
    })

    expect(report).toMatchObject({
      schemaVersion: 1,
      kind: 'spacewave.react-diagnostic-run',
      generatedAt: '2026-05-09T00:00:00.000Z',
      ok: true,
      tool: {
        name: 'react-doctor',
        version: '0.1.4',
        offline: true,
        exitCode: 0,
        signal: null,
        args: ['.', '--json', '--offline'],
      },
      error: null,
    })
  })
})
