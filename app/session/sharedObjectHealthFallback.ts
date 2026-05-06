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
  const classification = classifySharedObjectFallbackError(msg)

  return {
    status: SharedObjectHealthStatus.CLOSED,
    layer,
    commonReason: classification.commonReason,
    remediationHint: classification.remediationHint,
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

function classifySharedObjectFallbackError(message: string): {
  commonReason: SharedObjectHealthCommonReason
  remediationHint: SharedObjectHealthRemediationHint
} {
  const lower = message.toLowerCase()
  if (
    lower.includes('shared object initial state rejected') ||
    lower.includes('current key epoch missing')
  ) {
    return {
      commonReason: SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED,
      remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
    }
  }
  if (lower.includes('shared object not found')) {
    return {
      commonReason: SharedObjectHealthCommonReason.NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
    }
  }
  if (
    lower.includes('not a participant') ||
    lower.includes('no valid grant for our peer')
  ) {
    return {
      commonReason: SharedObjectHealthCommonReason.ACCESS_REVOKED,
      remediationHint: SharedObjectHealthRemediationHint.REQUEST_ACCESS,
    }
  }
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
