import { useEffect } from 'react'

import { readBrowserBootRecoveryStatus } from '@s4wave/app/prerender/boot-status.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type {
  BrowserBootRecoveryStatus,
  RuntimeAssetRecoveryStatus,
} from '@s4wave/sdk/status/status.pb.js'
import { webViewRootAssetStatusEvent } from '@aptre/bldr-react'

declare global {
  var __bldrWebViewRootAssetStatus:
    | {
        scriptPath: string
        status: number
        ok: boolean
        fetchSource?: string
        runtimeError?: string
        pluginAssetResult?: string
        contentType?: string
        bodyPrefix?: string
        classification: string
      }
    | undefined
}

// RecoveryStatusPublisher reports renderer recovery facts to the session
// whenever the boot or root asset status changes.
export function RecoveryStatusPublisher(props: { session: Session }) {
  const { session } = props

  useEffect(() => {
    // Reports still in flight when the session is released are abandoned.
    const ctrl = new AbortController()
    const publish = () => {
      const boot = readBootRecoveryStatus()
      const runtimeAsset = readRuntimeAssetRecoveryStatus()
      if (!boot && !runtimeAsset) return
      session.systemStatus
        .reportRecoveryStatus({ boot, runtimeAsset }, ctrl.signal)
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) return
          console.error('failed to publish runtime recovery status', err)
        })
    }

    publish()
    window.addEventListener('spacewave:boot-status', publish)
    window.addEventListener(webViewRootAssetStatusEvent, publish)
    return () => {
      ctrl.abort()
      window.removeEventListener('spacewave:boot-status', publish)
      window.removeEventListener(webViewRootAssetStatusEvent, publish)
    }
  }, [session])

  return null
}

function readBootRecoveryStatus(): BrowserBootRecoveryStatus | undefined {
  const status = readBrowserBootRecoveryStatus()
  if (!status) return undefined
  return {
    compatibilityVersion: status.compatibilityVersion,
    lastResetDecision: status.lastResetDecision,
    status: 'reported',
  }
}

function readRuntimeAssetRecoveryStatus():
  | RuntimeAssetRecoveryStatus
  | undefined {
  const status = globalThis.__bldrWebViewRootAssetStatus
  if (!status) return undefined
  return {
    scriptPath: status.scriptPath,
    statusCode: status.status,
    ok: status.ok,
    classification: status.classification,
    fetchSource: status.fetchSource,
    runtimeError: status.runtimeError,
    pluginAssetResult: status.pluginAssetResult,
    contentType: status.contentType,
    bodyPrefix: status.bodyPrefix,
    status: 'reported',
  }
}
