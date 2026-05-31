import { useCallback, useEffect } from 'react'

import { readBrowserBootRecoveryStatus } from '@s4wave/app/prerender/boot-status.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type {
  BrowserBootRecoveryStatus,
  RuntimeAssetRecoveryStatus,
} from '@s4wave/sdk/status/status.pb.js'
import { webViewRootAssetStatusEvent } from 'web/bldr-react/web-view-module-loader.js'

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

export function RecoveryStatusPublisher(props: { session: Session }) {
  const publish = useCallback(() => {
    const boot = readBootRecoveryStatus()
    const runtimeAsset = readRuntimeAssetRecoveryStatus()
    if (!boot && !runtimeAsset) return
    props.session.systemStatus
      .reportRecoveryStatus({ boot, runtimeAsset })
      .catch((err: unknown) => {
        console.error('failed to publish runtime recovery status', err)
      })
  }, [props.session])

  useEffect(() => {
    publish()
    window.addEventListener('spacewave:boot-status', publish)
    window.addEventListener(webViewRootAssetStatusEvent, publish)
    return () => {
      window.removeEventListener('spacewave:boot-status', publish)
      window.removeEventListener(webViewRootAssetStatusEvent, publish)
    }
  }, [publish])

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
