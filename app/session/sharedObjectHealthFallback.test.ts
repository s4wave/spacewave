import { describe, expect, it } from 'vitest'
import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'

import {
  buildSharedObjectFallbackHealth,
  getSharedObjectRouteHealth,
} from './sharedObjectHealthFallback.js'

describe('buildSharedObjectFallbackHealth', () => {
  it.each([
    {
      name: 'shared object not found',
      error: 'shared object not found',
      reason: SharedObjectHealthCommonReason.NOT_FOUND,
      hint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
    },
    {
      name: 'not a participant',
      error: 'not a participant',
      reason: SharedObjectHealthCommonReason.ACCESS_REVOKED,
      hint: SharedObjectHealthRemediationHint.REQUEST_ACCESS,
    },
    {
      name: 'access denied',
      error: 'access denied',
      reason: SharedObjectHealthCommonReason.ACCESS_REVOKED,
      hint: SharedObjectHealthRemediationHint.REQUEST_ACCESS,
    },
    {
      name: 'block missing',
      error: 'build cdn world engine: block not found',
      reason: SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
      hint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    },
    {
      name: 'transform config',
      error: 'decode transform config: invalid field',
      reason: SharedObjectHealthCommonReason.TRANSFORM_CONFIG_DECODE_FAILED,
      hint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    },
    {
      name: 'empty body type',
      error: 'empty shared object body type',
      reason: SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED,
      hint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    },
    {
      name: 'unsupported body type',
      error: 'unsupported shared object type: example/body',
      reason: SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED,
      hint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    },
    {
      name: 'unknown',
      error: 'local mount failed: disk offline',
      reason: SharedObjectHealthCommonReason.UNKNOWN,
      hint: SharedObjectHealthRemediationHint.NONE,
    },
  ])('preserves the current local fallback for $name', (tc) => {
    const health = buildSharedObjectFallbackHealth(
      new Error(tc.error),
      SharedObjectHealthLayer.SHARED_OBJECT,
    )

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason: tc.reason,
      remediationHint: tc.hint,
      error: tc.error,
    })
  })

  it('uses the body layer for SharedObject body access errors', () => {
    const health = buildSharedObjectFallbackHealth(
      new Error('build cdn world engine: block not found'),
      SharedObjectHealthLayer.BODY,
    )

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.BODY,
      commonReason: SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    })
  })
})

describe('getSharedObjectRouteHealth', () => {
  const watchedHealth: SharedObjectHealth = {
    status: SharedObjectHealthStatus.CLOSED,
    layer: SharedObjectHealthLayer.SHARED_OBJECT,
    commonReason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
    remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
    error: 'root signature validation failed',
  }

  it('keeps typed backend health ahead of mount-error fallback', () => {
    const health = getSharedObjectRouteHealth({
      mounted: false,
      bodyLoading: false,
      watchedHealth,
      mountError: new Error('shared object not found'),
      bodyError: null,
    })

    expect(health).toBe(watchedHealth)
  })

  it('keeps body access fallback ahead of typed backend health', () => {
    const health = getSharedObjectRouteHealth({
      mounted: true,
      bodyLoading: false,
      watchedHealth,
      mountError: null,
      bodyError: new Error('unsupported shared object type: example/body'),
    })

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.BODY,
      commonReason: SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    })
  })

  it('returns body loading health while the mounted SharedObject body loads', () => {
    const health = getSharedObjectRouteHealth({
      mounted: true,
      bodyLoading: true,
      watchedHealth: null,
      mountError: null,
      bodyError: null,
    })

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.LOADING,
      layer: SharedObjectHealthLayer.BODY,
      commonReason: SharedObjectHealthCommonReason.UNKNOWN,
      remediationHint: SharedObjectHealthRemediationHint.NONE,
      error: '',
    })
  })
})
