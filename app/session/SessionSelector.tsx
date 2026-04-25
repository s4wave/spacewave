import { useCallback } from 'react'
import { LuUser, LuChevronRight } from 'react-icons/lu'

import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { useSessionAccountStatuses } from '@s4wave/app/hooks/useSessionAccountStatuses.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { Button } from '@s4wave/web/ui/button.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { cn } from '@s4wave/web/style/utils.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'

// SessionSelector displays a full-page session picker for users with multiple sessions.
export function SessionSelector() {
  const resource = useSessionList()
  const accountStatuses = useSessionAccountStatuses()
  const navigate = useNavigate()
  const sessions = resource.value?.sessions ?? []

  const handleAddAccount = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])

  const handleHome = useCallback(() => {
    navigate({ path: '/landing' })
  }, [navigate])

  if (resource.loading) {
    return (
      <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Loading sessions',
              detail: 'Reading available sessions from the provider.',
            }}
          />
        </div>
      </div>
    )
  }

  if (resource.error) {
    return (
      <div className="bg-background-landing flex h-full w-full flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'error',
              title: 'Failed to load sessions',
              error: resource.error.message,
              onRetry: resource.retry,
            }}
          />
        </div>
      </div>
    )
  }

  if (sessions.length === 0) {
    return <NavigatePath to="/landing" replace />
  }

  return (
    <div className="bg-background-landing relative flex h-full w-full flex-col overflow-hidden">
      <ShootingStars className="pointer-events-none absolute inset-0 opacity-60" />

      <BackButton floating onClick={handleHome}>
        Home
      </BackButton>

      <div className="relative z-10 flex flex-1 flex-col items-center justify-center px-4">
        <AnimatedLogo followMouse={true} containerClassName="mb-6" />

        <h1 className="text-2xl font-bold tracking-wide">Welcome back</h1>
        <p className="text-foreground-alt/60 mb-6 text-sm">
          Choose a session to continue
        </p>

        <div className="w-full max-w-md space-y-2">
          {sessions.map((session) => (
            <SessionCard
              key={session.sessionIndex}
              session={session}
              accountStatus={accountStatuses.get(session.sessionIndex ?? 0)}
            />
          ))}
        </div>

        <div className="mt-6 flex items-center justify-center gap-3">
          <Button variant="outline" onClick={handleAddAccount}>
            Add account
          </Button>
        </div>
      </div>

      <div className="relative z-10 pb-3 text-center">
        <p className="text-foreground-alt/60 text-xs">
          local-first · encrypted
        </p>
      </div>
    </div>
  )
}

// SessionCard renders a single session entry in the selector list.
function SessionCard(props: {
  session: SessionListEntry
  accountStatus?: ProviderAccountStatus
}) {
  const navigate = useNavigate()
  const meta = useSessionMetadata(props.session.sessionIndex ?? null)
  const isCloudProvider = meta?.providerId === 'spacewave'
  const isLinked = meta?.providerId === 'local' && !!meta?.cloudAccountId
  const isInactive =
    props.accountStatus === ProviderAccountStatus.ProviderAccountStatus_DORMANT
  const title =
    meta?.displayName ||
    meta?.cloudEntityId ||
    `Session ${props.session.sessionIndex}`
  const providerLabel =
    meta?.providerDisplayName || (isCloudProvider ? 'Cloud' : 'Local')
  const subtitle =
    isCloudProvider && meta?.cloudEntityId && meta.cloudEntityId !== title ?
      `${providerLabel} · ${meta.cloudEntityId}`
    : !isCloudProvider && !meta?.displayName ?
      `${providerLabel} · Session ${props.session.sessionIndex}`
    : providerLabel

  const handleClick = useCallback(() => {
    navigate({ path: '/u/' + props.session.sessionIndex + '/' })
  }, [navigate, props.session.sessionIndex])

  return (
    <div
      onClick={handleClick}
      className={cn(
        'border-foreground/10 hover:bg-foreground/5 flex cursor-pointer items-center gap-3 rounded-lg border px-4 py-3 transition-colors',
        isLinked && 'opacity-50',
      )}
    >
      <div className="bg-foreground/5 flex h-9 w-9 items-center justify-center rounded-lg">
        <LuUser className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-foreground text-sm font-medium">{title}</span>
          {isCloudProvider && (
            <span className="bg-brand/15 text-brand rounded-full px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase">
              Cloud
            </span>
          )}
          {isLinked && (
            <span className="text-foreground-alt/80 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
              (linked)
            </span>
          )}
          {isInactive && (
            <span className="bg-foreground/6 text-foreground-alt/75 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
              (Inactive)
            </span>
          )}
        </div>
        <div className="text-foreground-alt/60 truncate text-xs">
          {subtitle}
        </div>
      </div>
      <LuChevronRight className="text-foreground-alt/40 h-4 w-4" />
    </div>
  )
}
