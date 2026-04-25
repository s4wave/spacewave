import { useEffect, useState } from 'react'
import {
  LuArrowRight,
  LuCircleCheck,
  LuLink,
  LuRefreshCw,
  LuX,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { PhaseChecklist } from './PhaseChecklist.js'
import type { Session } from '@s4wave/sdk/session/session.js'

export interface LinkDeviceDoneStepProps {
  session: Session | null | undefined
  remotePeerId: string | null
  onDone: () => void
  onLinkMore: () => void
}

type LinkSyncState = 'confirming' | 'syncing' | 'done'

// LinkDeviceDoneStep confirms the linked device and waits for the watched sync signal.
export function LinkDeviceDoneStep({
  session,
  remotePeerId,
  onDone,
  onLinkMore,
}: LinkDeviceDoneStepProps) {
  const [pairingConfirmed, setPairingConfirmed] = useState(false)
  const [pairingError, setPairingError] = useState<string | null>(null)
  const [syncState, setSyncState] = useState<LinkSyncState>('confirming')
  const syncDone = syncState === 'done'

  useEffect(() => {
    setPairingConfirmed(false)
    setPairingError(null)
    setSyncState('confirming')
    if (!session || !remotePeerId) return
    const controller = new AbortController()
    session
      .confirmPairing(remotePeerId, '', controller.signal)
      .then(() => {
        if (!controller.signal.aborted) {
          setPairingConfirmed(true)
          setSyncState('syncing')
        }
      })
      .catch((err: Error) => {
        if (!controller.signal.aborted) {
          setPairingError(err.message)
        }
      })
    return () => controller.abort()
  }, [session, remotePeerId])

  useEffect(() => {
    if (!session || !pairingConfirmed || !remotePeerId) return
    const controller = new AbortController()
    ;(async () => {
      for await (const resp of session.watchPairedDevices(controller.signal)) {
        if (controller.signal.aborted) break
        const devices = resp.pairedDevices ?? []
        if (devices.some((device) => device.peerId === remotePeerId)) {
          setSyncState('done')
          break
        }
      }
    })().catch(() => {})
    return () => {
      controller.abort()
    }
  }, [session, pairingConfirmed, remotePeerId])

  if (pairingError) {
    return (
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-3">
          <div className="bg-destructive/10 flex h-12 w-12 items-center justify-center rounded-full">
            <LuX className="text-destructive h-6 w-6" />
          </div>
          <h2 className="text-foreground text-sm font-medium">
            Pairing failed
          </h2>
          <p className="text-destructive text-xs">{pairingError}</p>
        </div>
        <button
          onClick={onLinkMore}
          className={cn(
            'w-full rounded-md border transition-all duration-300',
            'border-foreground/20 hover:border-foreground/40',
            'flex h-10 items-center justify-center gap-2',
          )}
        >
          <LuRefreshCw className="text-foreground-alt h-4 w-4" />
          <span className="text-foreground text-sm">Try again</span>
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-3">
        <div className="bg-brand/10 flex h-12 w-12 items-center justify-center rounded-full">
          {syncDone ?
            <LuCircleCheck className="text-brand h-6 w-6" />
          : <Spinner size="lg" className="text-brand" />}
        </div>
        <h2 className="text-foreground text-sm font-medium">
          {syncState === 'confirming' ?
            'Confirming linked device...'
          : syncDone ?
            'All set!'
          : 'Finishing device sync...'}
        </h2>
      </div>

      <PhaseChecklist
        phases={[
          {
            label: 'Pairing confirmed',
            done: pairingConfirmed,
            active: !pairingConfirmed,
          },
          {
            label: 'Syncing data',
            done: syncDone,
            active: pairingConfirmed && !syncDone,
          },
        ]}
      />

      {syncDone && (
        <div className="flex gap-2">
          <button
            onClick={onLinkMore}
            className={cn(
              'flex-1 rounded-md border transition-all duration-300',
              'border-foreground/20 hover:border-foreground/40',
              'flex h-10 items-center justify-center gap-2',
            )}
          >
            <LuLink className="text-foreground-alt h-4 w-4" />
            <span className="text-foreground text-sm">Link more</span>
          </button>
          <button
            onClick={onDone}
            className={cn(
              'flex-1 rounded-md border transition-all duration-300',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'flex h-10 items-center justify-center gap-2',
            )}
          >
            <span className="text-foreground text-sm">Dashboard</span>
            <LuArrowRight className="text-foreground-alt h-4 w-4" />
          </button>
        </div>
      )}

      {!syncDone && (
        <button
          onClick={onDone}
          className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
        >
          Skip and continue to dashboard
        </button>
      )}
    </div>
  )
}
