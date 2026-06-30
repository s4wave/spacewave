import { describe, test, expect, vi } from 'vitest'

import {
  applyMessagePortWorkerCommsOverride,
  type WorkerCommsDetectResult,
} from './worker-comms-detect.js'

function buildResult(
  config: WorkerCommsDetectResult['config'],
  overrides: Partial<WorkerCommsDetectResult['caps']> = {},
): WorkerCommsDetectResult {
  return {
    config,
    caps: {
      crossOriginIsolated: false,
      sabAvailable: false,
      opfsAvailable: false,
      webLocksAvailable: false,
      broadcastChannelAvailable: false,
      ...overrides,
    },
  }
}

describe('applyMessagePortWorkerCommsOverride', () => {
  test('passes through when no override is requested', () => {
    const result = buildResult('C', {
      crossOriginIsolated: true,
      sabAvailable: true,
    })
    expect(applyMessagePortWorkerCommsOverride(result, false)).toBe(result)
  })

  test('passes through when already Config A', () => {
    const result = buildResult('A')
    expect(applyMessagePortWorkerCommsOverride(result, true)).toBe(result)
  })

  test('forces Config A when COI/SAB are not actually available', () => {
    const result = buildResult('B', {
      crossOriginIsolated: false,
      sabAvailable: true,
    })
    const overridden = applyMessagePortWorkerCommsOverride(result, true)
    expect(overridden.config).toBe('A')
    expect(overridden.caps).toBe(result.caps)
  })

  test('refuses to force Config A over a genuine COI + SAB page', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = buildResult('C', {
      crossOriginIsolated: true,
      sabAvailable: true,
    })
    const overridden = applyMessagePortWorkerCommsOverride(result, true)
    expect(overridden).toBe(result)
    expect(overridden.config).toBe('C')
    expect(warn).toHaveBeenCalledTimes(1)
    warn.mockRestore()
  })

  test('refuses to force Config A over genuine COI + SAB even without OPFS', () => {
    const result = buildResult('B', {
      crossOriginIsolated: true,
      sabAvailable: true,
      opfsAvailable: false,
    })
    const overridden = applyMessagePortWorkerCommsOverride(result, true)
    expect(overridden.config).toBe('B')
  })
})
