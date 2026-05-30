import { describe, expect, it } from 'vitest'
import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import { SharedObjectHealthError } from '@s4wave/sdk/sobject/sobject.js'

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
      name: 'initial state rejected',
      error: 'initial state pull: shared object initial state rejected',
      reason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
      hint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
    },
    {
      name: 'current key epoch missing',
      error: 'rejoin: current key epoch missing for self-enroll recovery',
      reason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
      hint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
    },
    {
      name: 'not a participant',
      error: 'access denied: peer is not a participant',
      reason: SharedObjectHealthCommonReason.ACCESS_REVOKED,
      hint: SharedObjectHealthRemediationHint.REQUEST_ACCESS,
    },
    {
      name: 'no valid grant',
      error: 'access denied: no valid grant for our peer',
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
      name: 'unknown',
      error: 'local mount failed: disk offline',
      reason: SharedObjectHealthCommonReason.UNKNOWN,
      hint: SharedObjectHealthRemediationHint.NONE,
    },
  ])('matches backend health vocabulary for $name', (tc) => {
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

  it('leaves generic access denied errors as unknown without a known SharedObject marker', () => {
    const health = buildSharedObjectFallbackHealth(
      new Error('access denied'),
      SharedObjectHealthLayer.SHARED_OBJECT,
    )

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason: SharedObjectHealthCommonReason.UNKNOWN,
      remediationHint: SharedObjectHealthRemediationHint.NONE,
      error: 'access denied',
    })
  })

  it('leaves untyped body configuration strings as unknown after typed body responses own them', () => {
    const health = buildSharedObjectFallbackHealth(
      new Error('unsupported shared object type: example/body'),
      SharedObjectHealthLayer.BODY,
    )

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.BODY,
      commonReason: SharedObjectHealthCommonReason.UNKNOWN,
      remediationHint: SharedObjectHealthRemediationHint.NONE,
      error: 'unsupported shared object type: example/body',
    })
  })

  it('preserves typed SDK SharedObject health before string fallback classification', () => {
    const typedHealth: SharedObjectHealth = {
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.BODY,
      commonReason: SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
      error: 'opaque transport message',
    }
    const health = buildSharedObjectFallbackHealth(
      new SharedObjectHealthError(typedHealth),
      SharedObjectHealthLayer.SHARED_OBJECT,
    )

    expect(health).toBe(typedHealth)
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
      bodyError: new Error('build cdn world engine: block not found'),
    })

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.BODY,
      commonReason: SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    })
  })

  it('keeps body access fallback ahead of body loading health', () => {
    const health = getSharedObjectRouteHealth({
      mounted: true,
      bodyLoading: true,
      watchedHealth: null,
      mountError: null,
      bodyError: new Error('build cdn world engine: block not found'),
    })

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.BODY,
      commonReason: SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    })
  })

  it('preserves typed CDN pointer loading health from body response outcomes', () => {
    const typedHealth: SharedObjectHealth = {
      status: SharedObjectHealthStatus.LOADING,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason: SharedObjectHealthCommonReason.UNKNOWN,
      remediationHint: SharedObjectHealthRemediationHint.NONE,
      error: '',
    }
    const health = getSharedObjectRouteHealth({
      mounted: true,
      bodyLoading: true,
      watchedHealth: null,
      mountError: null,
      bodyError: new SharedObjectHealthError(typedHealth),
    })

    expect(health).toBe(typedHealth)
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

  it('keeps body loading ahead of watched health and mount-error fallback', () => {
    const health = getSharedObjectRouteHealth({
      mounted: true,
      bodyLoading: true,
      watchedHealth,
      mountError: new Error('shared object not found'),
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

  it('uses mount-error fallback after body and watched health are absent', () => {
    const health = getSharedObjectRouteHealth({
      mounted: false,
      bodyLoading: false,
      watchedHealth: null,
      mountError: new Error('shared object not found'),
      bodyError: null,
    })

    expect(health).toMatchObject({
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason: SharedObjectHealthCommonReason.NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
    })
  })
})
