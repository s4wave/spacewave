import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import type { LoadingState, LoadingView } from '@s4wave/web/ui/loading/types.js'

// toSharedObjectView maps a SharedObjectHealth snapshot onto a LoadingView.
// LOADING -> 'active' (work in progress), READY -> 'synced', DEGRADED /
// CLOSED / UNKNOWN -> 'error'. The remediation hint becomes the detail line;
// the raw error message populates the error box when present.
export function toSharedObjectView(
  health: SharedObjectHealth | null,
  options: { onRetry?: () => void; onCancel?: () => void } = {},
): LoadingView {
  if (!health || health.status === SharedObjectHealthStatus.UNKNOWN) {
    return {
      state: 'loading',
      title: 'Preparing shared object',
      detail: 'Waiting for shared object health.',
    }
  }
  const summary = summariseHealth(health)
  const state: LoadingState =
    summary.state === 'active' ? 'active'
    : summary.state === 'synced' ? 'synced'
    : 'error'
  if (state === 'error') {
    return {
      state,
      title: summary.title,
      detail: summary.detail,
      error: health.error || undefined,
      onRetry: options.onRetry,
      onCancel: options.onCancel,
    }
  }
  return {
    state,
    title: summary.title,
    detail: summary.detail,
  }
}

interface HealthSummary {
  state: 'active' | 'synced' | 'error'
  title: string
  detail: string
}

function summariseHealth(health: SharedObjectHealth): HealthSummary {
  if (health.status === SharedObjectHealthStatus.LOADING) {
    if (health.layer === SharedObjectHealthLayer.BODY) {
      return {
        state: 'active',
        title: 'Mounting shared object body',
        detail:
          'The shared object is available, and the body content is still loading.',
      }
    }
    return {
      state: 'active',
      title: 'Mounting shared object',
      detail: 'Checking availability and preparing the shared object for use.',
    }
  }
  if (health.status === SharedObjectHealthStatus.READY) {
    return {
      state: 'synced',
      title: 'Shared object ready',
      detail: 'Mounted and available.',
    }
  }
  if (health.status === SharedObjectHealthStatus.DEGRADED) {
    return {
      state: 'error',
      title: 'Shared object degraded',
      detail:
        'The shared object is partially available, but Alpha detected a recoverable problem.',
    }
  }
  return summariseClosed(health)
}

function summariseClosed(health: SharedObjectHealth): HealthSummary {
  switch (health.commonReason) {
    case SharedObjectHealthCommonReason.NOT_FOUND:
      return {
        state: 'error',
        title: 'Shared object not found',
        detail:
          'This shared object is no longer available from the current account or provider.',
      }
    case SharedObjectHealthCommonReason.ACCESS_REVOKED:
      return {
        state: 'error',
        title: 'Access revoked',
        detail:
          'The current session is no longer allowed to read this shared object.',
      }
    case SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED:
      return {
        state: 'error',
        title: 'Initial state rejected',
        detail:
          'The shared object state failed verification. The owner needs to repair or republish it.',
      }
    case SharedObjectHealthCommonReason.BLOCK_NOT_FOUND:
      return {
        state: 'error',
        title: 'Required block missing',
        detail:
          'A block required to mount this shared object could not be found.',
      }
    case SharedObjectHealthCommonReason.TRANSFORM_CONFIG_DECODE_FAILED:
      return {
        state: 'error',
        title: 'Transform configuration invalid',
        detail: 'The transform configuration could not be decoded.',
      }
    case SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED:
      return {
        state: 'error',
        title: 'Body configuration invalid',
        detail:
          'The shared object body metadata could not be decoded into a supported view.',
      }
    default:
      return {
        state: 'error',
        title:
          health.layer === SharedObjectHealthLayer.BODY ?
            'Shared object body failed'
          : 'Shared object unavailable',
        detail:
          health.layer === SharedObjectHealthLayer.BODY ?
            'The shared object opened, but the body content could not be mounted.'
          : 'Alpha could not mount this shared object.',
      }
  }
}
