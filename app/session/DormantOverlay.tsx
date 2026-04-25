import { useCallback, useState } from 'react'
import { LuMoon, LuSparkles, LuUser } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import { findPersonalCanceledBillingAccount } from '../provider/spacewave/resubscribe-target.js'

export interface DormantOverlayProps {
  metadata: SessionMetadata
}

// DormantOverlay renders a full-page gate for cloud sessions whose account has
// entered the DORMANT state (subscription inactive or RBAC denied). Offers
// resubscribe as the primary action and returning to the session list as the
// secondary action.
export function DormantOverlay({ metadata }: DormantOverlayProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { accountId } = useSessionInfo(session)
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const [error, setError] = useState<string | null>(null)
  const [resolving, setResolving] = useState(false)

  const handleBack = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])

  const handleReactivate = useCallback(async () => {
    if (!session) return

    setResolving(true)
    setError(null)
    try {
      const resp = await session.spacewave.listManagedBillingAccounts()
      const target = findPersonalCanceledBillingAccount(
        resp.accounts ?? [],
        accountId,
      )
      if (target?.id) {
        navigateSession({ path: `billing/${target.id}?reactivate=1` })
        return
      }
      navigateSession({ path: 'plan/no-active' })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setResolving(false)
    }
  }, [accountId, navigateSession, session])

  const handleOpenLocal = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])

  const entityId = metadata.displayName || metadata.cloudEntityId || ''
  const providerLabel = metadata.providerDisplayName || 'Cloud'

  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 overflow-y-auto p-6 outline-none md:p-10">
      <BackButton floating onClick={handleBack}>
        Sessions
      </BackButton>

      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />

      <div className="relative z-10 flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="text-foreground mt-2 text-xl font-bold tracking-wide">
            Subscription inactive
          </h1>
          {entityId && (
            <div className="flex items-center gap-2">
              <span className="text-foreground text-sm font-medium">
                {entityId}
              </span>
              <span className="bg-warning/15 text-warning rounded-full px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase">
                {providerLabel}
              </span>
            </div>
          )}
        </div>

        <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
            <div className="flex flex-col items-center gap-2">
              <div className="bg-warning/10 flex h-12 w-12 items-center justify-center rounded-full">
                <LuMoon className="text-warning h-6 w-6" />
              </div>
            </div>

            <p className="text-foreground-alt text-center text-sm leading-relaxed">
              This cloud session is paused because the subscription is not
              active. Reactivate the subscription to continue, or open a local
              session in the meantime.
            </p>

            <button
              onClick={handleReactivate}
              disabled={resolving}
              className={cn(
                'group w-full rounded-md border transition-all duration-300',
                'border-brand/30 bg-brand/10 hover:bg-brand/20',
                'flex h-10 items-center justify-center gap-2',
              )}
            >
              <LuSparkles className="h-4 w-4" />
              <span className="text-foreground text-sm">
                {resolving ? 'Opening billing...' : 'Reactivate subscription'}
              </span>
            </button>

            {error && (
              <p className="text-destructive text-center text-xs leading-relaxed">
                {error}
              </p>
            )}

            <button
              onClick={handleOpenLocal}
              className={cn(
                'group w-full rounded-md border transition-all duration-300',
                'border-foreground/10 bg-foreground/5 hover:bg-foreground/10',
                'flex h-10 items-center justify-center gap-2',
              )}
            >
              <LuUser className="h-4 w-4" />
              <span className="text-foreground text-sm">
                Open a local session
              </span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
