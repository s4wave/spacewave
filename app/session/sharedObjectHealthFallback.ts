import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import { getSharedObjectHealthFromError } from '@s4wave/sdk/sobject/sobject.js'

export interface SharedObjectRouteHealthInput {
  mounted: boolean
  bodyLoading: boolean
  watchedHealth: SharedObjectHealth | null | undefined
  mountError: Error | null | undefined
  bodyError: Error | null | undefined
}

export function getSharedObjectRouteHealth({
  mounted,
  bodyLoading,
  watchedHealth,
  mountError,
  bodyError,
}: SharedObjectRouteHealthInput): SharedObjectHealth | null {
  if (bodyError) {
    return buildSharedObjectFallbackHealth(
      bodyError,
      SharedObjectHealthLayer.BODY,
    )
  }
  if (mounted && bodyLoading) {
    return buildSharedObjectLoadingHealth(SharedObjectHealthLayer.BODY)
  }
  if (watchedHealth) {
    return watchedHealth
  }
  if (mountError) {
    return buildSharedObjectFallbackHealth(
      mountError,
      SharedObjectHealthLayer.SHARED_OBJECT,
    )
  }
  return null
}

export function buildSharedObjectFallbackHealth(
  err: Error,
  layer: SharedObjectHealthLayer,
): SharedObjectHealth {
  const typedHealth = getSharedObjectHealthFromError(err)
  if (typedHealth) {
    return typedHealth
  }

  const msg = err.message || 'unknown shared object error'

  return {
    status: SharedObjectHealthStatus.CLOSED,
    layer,
    commonReason: SharedObjectHealthCommonReason.UNKNOWN,
    remediationHint: SharedObjectHealthRemediationHint.NONE,
    error: msg,
  }
}

export function buildSharedObjectLoadingHealth(
  layer: SharedObjectHealthLayer,
): SharedObjectHealth {
  return {
    status: SharedObjectHealthStatus.LOADING,
    layer,
    commonReason: SharedObjectHealthCommonReason.UNKNOWN,
    remediationHint: SharedObjectHealthRemediationHint.NONE,
    error: '',
  }
}
