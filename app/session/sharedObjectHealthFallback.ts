import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'

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
  const msg = err.message || 'unknown shared object error'
  const lower = msg.toLowerCase()
  const reason = getSharedObjectFallbackReason(lower)

  if (lower.includes('shared object not found')) {
    return {
      status: SharedObjectHealthStatus.CLOSED,
      layer,
      commonReason: SharedObjectHealthCommonReason.NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
      error: msg,
    }
  }
  if (lower.includes('not a participant') || lower.includes('access denied')) {
    return {
      status: SharedObjectHealthStatus.CLOSED,
      layer,
      commonReason: SharedObjectHealthCommonReason.ACCESS_REVOKED,
      remediationHint: SharedObjectHealthRemediationHint.REQUEST_ACCESS,
      error: msg,
    }
  }
  return {
    status: SharedObjectHealthStatus.CLOSED,
    layer,
    commonReason: reason.commonReason,
    remediationHint: reason.remediationHint,
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

function getSharedObjectFallbackReason(lower: string): {
  commonReason: SharedObjectHealthCommonReason
  remediationHint: SharedObjectHealthRemediationHint
} {
  if (lower.includes('block not found')) {
    return {
      commonReason: SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    }
  }
  if (lower.includes('transform config')) {
    return {
      commonReason:
        SharedObjectHealthCommonReason.TRANSFORM_CONFIG_DECODE_FAILED,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    }
  }
  if (
    lower.includes('empty shared object body type') ||
    lower.includes('unsupported shared object type')
  ) {
    return {
      commonReason: SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    }
  }
  return {
    commonReason: SharedObjectHealthCommonReason.UNKNOWN,
    remediationHint: SharedObjectHealthRemediationHint.NONE,
  }
}
