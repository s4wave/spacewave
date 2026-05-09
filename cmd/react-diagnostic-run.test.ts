import { describe, expect, it } from 'vitest'

import {
  buildReactDiagnosticRunReport,
  buildReactDoctorArgs,
  formatReactDiagnosticHumanSummary,
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
      fullAudit: false,
      passthroughArgs: [],
    })
    expect(buildReactDoctorArgs(options)).toEqual([
      '.',
      '--json',
      '--offline',
      '--no-dead-code',
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
      fullAudit: false,
      passthroughArgs: ['--project', 'spacewave', '--diff', 'master'],
    })
    expect(buildReactDoctorArgs(options)).toEqual([
      'app',
      '--json',
      '--no-dead-code',
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
      '--no-dead-code',
      '--fail-on',
      'warning',
    ])
  })

  it('enables full dead-code diagnostics only for manual full audit mode', () => {
    const options = parseReactDiagnosticRunArgs([
      '--full-audit',
      '--output',
      '.tmp/react-diagnostic-full-audit.json',
    ])

    expect(options).toEqual({
      directory: '.',
      outputPath: '.tmp/react-diagnostic-full-audit.json',
      offline: true,
      compact: false,
      fullAudit: true,
      passthroughArgs: [],
    })
    expect(buildReactDoctorArgs(options)).toEqual([
      '.',
      '--json',
      '--offline',
      '--full',
      '--dead-code',
      '--fail-on',
      'none',
    ])
  })

  it('allows explicit React Doctor dead-code flags outside full audit mode', () => {
    const options = parseReactDiagnosticRunArgs(['--dead-code'])

    expect(buildReactDoctorArgs(options)).toEqual([
      '.',
      '--json',
      '--offline',
      '--fail-on',
      'none',
      '--dead-code',
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
        summary: {
          errorCount: 0,
          warningCount: 0,
          affectedFileCount: 0,
          totalDiagnosticCount: 0,
          score: 100,
          scoreLabel: 'Excellent',
        },
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

  it('formats a concise human summary for file output', () => {
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
        diagnostics: [
          {
            filePath: 'app/view.tsx',
            plugin: 'react-doctor',
            rule: 'memoized-context-value',
            severity: 'warning',
            message:
              'Context provider value is recreated every render, causing consumers to rerender.',
            help: 'Wrap the provider value in useMemo.',
            line: 42,
            column: 11,
            category: 'Performance',
          },
        ],
        summary: {
          errorCount: 0,
          warningCount: 1,
          affectedFileCount: 1,
          totalDiagnosticCount: 1,
          score: 82,
          scoreLabel: 'Good',
        },
        error: null,
      },
      parseError: null,
    })

    const summary = formatReactDiagnosticHumanSummary(
      report,
      '.tmp/react-diagnostic.json',
    )

    expect(summary).toContain('React Doctor completed with diagnostics')
    expect(summary).toContain('Mode: full (offline)')
    expect(summary).toContain('Summary: 0 errors, 1 warning, 1 affected file, 82/100 Good')
    expect(summary).toContain('- warning react-doctor/memoized-context-value (1 site, Performance)')
    expect(summary).toContain('app/view.tsx:42')
    expect(summary).toContain('Machine contract: schema-versioned JSON report.')
  })

  it('formats partial React Doctor reports without throwing', () => {
    const report = buildReactDiagnosticRunReport({
      directory: '.',
      offline: true,
      args: ['.', '--json', '--offline'],
      generatedAt: '2026-05-09T00:00:00.000Z',
      processResult: {
        code: 1,
        signal: null,
        stdout: '{}',
        stderr: 'react doctor failed',
      },
      reactDoctor: {
        schemaVersion: 1,
        ok: false,
        directory: '/repo',
        mode: 'full',
        error: { message: 'react doctor failed' },
      },
      parseError: null,
    })

    const summary = formatReactDiagnosticHumanSummary(
      report,
      '.tmp/react-diagnostic.json',
    )

    expect(summary).toContain('React Doctor failed')
    expect(summary).toContain('Summary: 0 errors, 0 warnings, 0 affected files, score unavailable')
    expect(summary).toContain('react doctor failed')
  })
})
